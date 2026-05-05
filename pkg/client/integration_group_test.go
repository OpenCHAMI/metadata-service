// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	group "github.com/OpenCHAMI/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/openchami/fabrica/pkg/fabrica"

	"github.com/OpenCHAMI/metadata-service/pkg/client"
	"github.com/stretchr/testify/require"
)

func isServerAvailable(url string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(url + "/openapi.yaml")
	if err != nil {
		return false
	}
	defer resp.Body.Close() // nolint:errcheck
	return resp.StatusCode == http.StatusOK
}

func skipIfServerUnavailable(t *testing.T, baseURL string) {
	if !isServerAvailable(baseURL) {
		t.Skipf("Test server at %s is not available, skipping integration test", baseURL)
	}
}

func startTestServer() string {
	// Assumes server is running at localhost:27777
	return "http://localhost:27777"
}

func TestGroupTemplateValidation(t *testing.T) {
	baseURL := startTestServer()
	skipIfServerUnavailable(t, baseURL)

	c, _ := client.NewClient(baseURL, &http.Client{Timeout: 5 * time.Second})
	ctx := context.Background()

	// 1. Create group with missing required variable
	badTemplate := "#cloud-config\nhostname: {{missing_var}}"
	reqBad := client.CreateGroupRequest{
		Metadata: fabrica.Metadata{Name: "test-bad"},
		Spec: group.GroupSpec{
			Template: badTemplate,
			MetaData: map[string]string{"hostname": "test-host"},
		},
	}
	_, err := c.CreateGroup(ctx, reqBad)
	require.Error(t, err, "Expected validation error for missing variable")

	// 2. Create group with all required variables present
	goodTemplate := "#cloud-config\nhostname: {{hostname}}"
	reqGood := client.CreateGroupRequest{
		Metadata: fabrica.Metadata{Name: "test-good"},
		Spec: group.GroupSpec{
			Template: goodTemplate,
			MetaData: map[string]string{"hostname": "test-host"},
		},
	}
	created, err := c.CreateGroup(ctx, reqGood)
	require.NoError(t, err, "Expected successful creation with valid template")
	require.True(t, created.Status.Valid, "Status.Valid should be true")

	// 3. Update group with invalid YAML after rendering
	invalidYAML := "#cloud-config\nhostname: {{hostname}}\nfoo: ["
	reqUpdate := client.UpdateGroupRequest{
		Metadata: fabrica.Metadata{Name: "test-good"},
		Spec: group.GroupSpec{
			Template: invalidYAML,
			MetaData: map[string]string{"hostname": "test-host"},
		},
	}
	_, err = c.UpdateGroup(ctx, created.Metadata.UID, reqUpdate)
	require.Error(t, err, "Expected validation error for invalid YAML")

	// 4. Bulk validation: create multiple groups, some valid, some invalid
	for i := 0; i < 3; i++ {
		tmpl := goodTemplate
		name := fmt.Sprintf("bulk-%d", i)
		if i == 2 {
			tmpl = badTemplate
			name = "bulk-bad"
		}
		req := client.CreateGroupRequest{
			Metadata: fabrica.Metadata{Name: name},
			Spec: group.GroupSpec{
				Template: tmpl,
				MetaData: map[string]string{"hostname": "bulk-host"},
			},
		}
		_, err := c.CreateGroup(ctx, req)
		if i == 2 {
			require.Error(t, err, "Expected error for bulk invalid group")
		} else {
			require.NoError(t, err, "Expected success for bulk valid group")
		}
	}
}

// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package handlers_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cloudinitv1 "github.com/OpenCHAMI/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/metadata-service/pkg/handlers"
	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

type integrationStore struct {
	clusterDefaults *handlers.ClusterDefaults
	instanceInfo    map[string]*handlers.InstanceInfo
	groupData       map[string]*cloudinitv1.Group
}

func (m *integrationStore) GetClusterDefaults() (*handlers.ClusterDefaults, error) {
	if m.clusterDefaults == nil {
		return nil, fmt.Errorf("no cluster defaults")
	}
	return m.clusterDefaults, nil
}

func (m *integrationStore) GetInstanceInfo(id string) (*handlers.InstanceInfo, error) {
	if info, ok := m.instanceInfo[id]; ok {
		return info, nil
	}
	return nil, fmt.Errorf("no instance info for %s", id)
}

func (m *integrationStore) GetGroupData(name string) (*cloudinitv1.Group, error) {
	if data, ok := m.groupData[name]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("no data for group %s", name)
}

func newIntegrationServer(t *testing.T) (*httptest.Server, *smdclient.MockSMDClient, *integrationStore) {
	t.Helper()

	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:ff",
		IP:   "10.0.0.100",
	})

	store := &integrationStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:          "http://placeholder",
			CloudProvider:    "OpenCHAMI",
			Region:           "us-test-1",
			AvailabilityZone: "test-az-1",
			ClusterName:      "testcluster",
			ShortName:        "tc",
			NidLength:        4,
			PublicKeys:       []string{"ssh-rsa AAAAB3... default@key"},
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*cloudinitv1.Group{
			"compute": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Compute nodes",
					Template:    "#cloud-config\npackages:\n  - git\n",
					MetaData: map[string]string{
						"custom_key": "custom_value",
					},
				},
			},
			"ntp": {
				Spec: cloudinitv1.GroupSpec{
					Description: "NTP config",
					Template:    "#cloud-config\nntp:\n  servers: [\"ntp.local\"]\n",
				},
			},
			"rack1": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Rack 1 metadata only",
					Template:    "",
					MetaData: map[string]string{
						"syslog_forwarder": "syslog.rack1.local",
					},
				},
			},
		},
	}

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "ntp", "rack1"})

	r := chi.NewRouter()
	r.Get("/meta-data", handlers.MetaDataHandler(smd, store))
	r.Get("/vendor-data", handlers.VendorDataHandler(smd, store))
	r.Get("/{group}.yaml", handlers.GroupUserDataHandler(smd, store))

	srv := httptest.NewServer(r)
	store.clusterDefaults.BaseURL = srv.URL
	return srv, smd, store
}

func TestIntegrationCloudInitFlow(t *testing.T) {
	srv, _, _ := newIntegrationServer(t)
	defer srv.Close()

	client := &http.Client{}

	metaReq, err := http.NewRequest("GET", srv.URL+"/meta-data", nil)
	if err != nil {
		t.Fatalf("failed to build meta-data request: %v", err)
	}
	metaReq.Header.Set("X-Forwarded-For", "10.0.0.100")

	metaResp, err := client.Do(metaReq)
	if err != nil {
		t.Fatalf("meta-data request failed: %v", err)
	}
	defer func() {
		if err := metaResp.Body.Close(); err != nil {
			t.Errorf("failed to close meta-data response body: %v", err)
		}
	}()

	if metaResp.StatusCode != http.StatusOK {
		t.Fatalf("meta-data status %d", metaResp.StatusCode)
	}

	metaBody, err := io.ReadAll(metaResp.Body)
	if err != nil {
		t.Fatalf("meta-data read failed: %v", err)
	}

	var metadata handlers.MetaData
	if err := yaml.Unmarshal(metaBody, &metadata); err != nil {
		t.Fatalf("meta-data YAML parse failed: %v", err)
	}

	if metadata.InstanceID != "x1000c0s0b0n0" {
		t.Fatalf("unexpected instance-id: %s", metadata.InstanceID)
	}

	if _, ok := metadata.InstanceData.V1.VendorData.Groups["compute"]; !ok {
		t.Fatalf("expected compute group in vendor_data")
	}
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["ntp"]; !ok {
		t.Fatalf("expected ntp group in vendor_data")
	}
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["rack1"]; !ok {
		t.Fatalf("expected rack1 group in vendor_data")
	}

	vendorReq, err := http.NewRequest("GET", srv.URL+"/vendor-data", nil)
	if err != nil {
		t.Fatalf("failed to build vendor-data request: %v", err)
	}
	vendorReq.Header.Set("X-Forwarded-For", "10.0.0.100")

	vendorResp, err := client.Do(vendorReq)
	if err != nil {
		t.Fatalf("vendor-data request failed: %v", err)
	}
	defer func() {
		if err := vendorResp.Body.Close(); err != nil {
			t.Errorf("failed to close vendor-data response body: %v", err)
		}
	}()

	if vendorResp.StatusCode != http.StatusOK {
		t.Fatalf("vendor-data status %d", vendorResp.StatusCode)
	}

	vendorBody, err := io.ReadAll(vendorResp.Body)
	if err != nil {
		t.Fatalf("vendor-data read failed: %v", err)
	}

	vendorText := string(vendorBody)
	if !strings.HasPrefix(vendorText, "#include\n") {
		t.Fatalf("vendor-data does not start with #include")
	}
	if !strings.Contains(vendorText, srv.URL+"/compute.yaml") {
		t.Fatalf("vendor-data missing compute include")
	}
	if !strings.Contains(vendorText, srv.URL+"/ntp.yaml") {
		t.Fatalf("vendor-data missing ntp include")
	}
	if strings.Contains(vendorText, srv.URL+"/rack1.yaml") {
		t.Fatalf("vendor-data should not include rack1 because template is empty")
	}

	groupReq, err := http.NewRequest("GET", srv.URL+"/compute.yaml", nil)
	if err != nil {
		t.Fatalf("failed to build group request: %v", err)
	}
	groupReq.Header.Set("X-Forwarded-For", "10.0.0.100")

	groupResp, err := client.Do(groupReq)
	if err != nil {
		t.Fatalf("group request failed: %v", err)
	}
	defer func() {
		if err := groupResp.Body.Close(); err != nil {
			t.Errorf("failed to close group response body: %v", err)
		}
	}()

	if groupResp.StatusCode != http.StatusOK {
		t.Fatalf("group status %d", groupResp.StatusCode)
	}

	groupBody, err := io.ReadAll(groupResp.Body)
	if err != nil {
		t.Fatalf("group read failed: %v", err)
	}

	if !strings.Contains(string(groupBody), "packages:") {
		t.Fatalf("group response missing expected template content")
	}
}

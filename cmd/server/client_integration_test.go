// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http/httptest"
	"testing"

	v1 "github.com/OpenCHAMI/cloud-init/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/cloud-init/internal/storage"
	"github.com/OpenCHAMI/cloud-init/pkg/client"
	"github.com/go-chi/chi/v5"
	"github.com/openchami/fabrica/pkg/fabrica"
	"github.com/stretchr/testify/require"
)

func TestClientLibraryAgainstServerRoutes(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, storage.InitFileBackend(dataDir))
	require.NoError(t, registerResourcePrefixes())

	r := chi.NewRouter()
	RegisterGeneratedRoutes(r)

	srv := httptest.NewServer(r)
	defer srv.Close()

	c, err := client.NewClient(srv.URL, srv.Client())
	require.NoError(t, err)

	ctx := context.Background()

	createReq := client.CreateGroupRequest{
		Metadata: fabrica.Metadata{Name: "client-test"},
		Spec: v1.GroupSpec{
			Template: "#cloud-config\npackages:\n  - git\n",
			MetaData: map[string]string{"custom_key": "value"},
		},
	}
	created, err := c.CreateGroup(ctx, createReq)
	require.NoError(t, err)
	require.NotEmpty(t, created.Metadata.UID)

	got, err := c.GetGroup(ctx, created.Metadata.UID)
	require.NoError(t, err)
	require.Equal(t, "client-test", got.Metadata.Name)

	updateReq := client.UpdateGroupRequest{
		Metadata: fabrica.Metadata{Name: "client-test"},
		Spec: v1.GroupSpec{
			Template: "#cloud-config\npackages:\n  - curl\n",
			MetaData: map[string]string{"custom_key": "updated"},
		},
	}
	updated, err := c.UpdateGroup(ctx, created.Metadata.UID, updateReq)
	require.NoError(t, err)
	require.Equal(t, "updated", updated.Spec.MetaData["custom_key"])
}

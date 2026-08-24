// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openchami/fabrica/pkg/fabrica"
	v1 "github.com/openchami/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/openchami/metadata-service/internal/storage"
)

func initTestStorageBackend(t *testing.T) {
	t.Helper()

	if err := storage.InitFileBackend(t.TempDir()); err != nil {
		t.Fatalf("InitFileBackend failed: %v", err)
	}
}

func saveClusterDefaultsFixture(t *testing.T, uid, name, baseURL string, updatedAt time.Time) {
	t.Helper()

	resource := &v1.ClusterDefaults{
		APIVersion: "v1",
		Kind:       "ClusterDefaults",
		Metadata: fabrica.Metadata{
			Name:      name,
			UID:       uid,
			CreatedAt: updatedAt,
			UpdatedAt: updatedAt,
		},
		Spec: v1.ClusterDefaultsSpec{
			BaseURL:     baseURL,
			ClusterName: name,
		},
	}

	if err := storage.SaveClusterDefaults(context.Background(), resource); err != nil {
		t.Fatalf("SaveClusterDefaults failed: %v", err)
	}
}

func saveInstanceInfoFixture(t *testing.T, uid, name, instanceID, hostname string, updatedAt time.Time) {
	t.Helper()

	resource := &v1.InstanceInfo{
		APIVersion: "v1",
		Kind:       "InstanceInfo",
		Metadata: fabrica.Metadata{
			Name:      name,
			UID:       uid,
			CreatedAt: updatedAt,
			UpdatedAt: updatedAt,
		},
		Spec: v1.InstanceInfoSpec{
			InstanceID: instanceID,
			Hostname:   hostname,
		},
	}

	if err := storage.SaveInstanceInfo(context.Background(), resource); err != nil {
		t.Fatalf("SaveInstanceInfo failed: %v", err)
	}
}

func saveGroupFixture(t *testing.T, uid, name, description, template string, updatedAt time.Time) {
	t.Helper()

	resource := &v1.Group{
		APIVersion: "v1",
		Kind:       "Group",
		Metadata: fabrica.Metadata{
			Name:      name,
			UID:       uid,
			CreatedAt: updatedAt,
			UpdatedAt: updatedAt,
		},
		Spec: v1.GroupSpec{
			Description: description,
			Template:    template,
		},
	}

	if err := storage.SaveGroup(context.Background(), resource); err != nil {
		t.Fatalf("SaveGroup failed: %v", err)
	}
}

func TestStorageAdapterGetClusterDefaultsReturnsLatestUpdated(t *testing.T) {
	initTestStorageBackend(t)

	now := time.Now().UTC()
	saveClusterDefaultsFixture(t, "clusterdefaults-old", "older", "http://old.example", now.Add(-time.Minute))
	saveClusterDefaultsFixture(t, "clusterdefaults-new", "newer", "http://new.example", now)

	got, err := NewStorageAdapter().GetClusterDefaults()
	if err != nil {
		t.Fatalf("GetClusterDefaults returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected cluster defaults, got nil")
		return
	}
	if got.BaseURL != "http://new.example" {
		t.Fatalf("expected latest cluster defaults base URL, got %q", got.BaseURL)
	}
}

func TestStorageAdapterGetInstanceInfoMatchesByNameOrSpecAndPrefersLatest(t *testing.T) {
	initTestStorageBackend(t)

	now := time.Now().UTC()
	saveInstanceInfoFixture(t, "instanceinfo-fallback", "custom-record", "x1000c0s0b0n0", "fallback-host", now.Add(-time.Minute))
	saveInstanceInfoFixture(t, "instanceinfo-latest", "x1000c0s0b0n0", "x1000c0s0b0n0", "latest-host", now)

	got, err := NewStorageAdapter().GetInstanceInfo("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("GetInstanceInfo returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected instance info, got nil")
		return
	}
	if got.Hostname != "latest-host" {
		t.Fatalf("expected latest hostname, got %q", got.Hostname)
	}
	if got.InstanceID != "x1000c0s0b0n0" {
		t.Fatalf("expected matching instance ID, got %q", got.InstanceID)
	}
}

func TestStorageAdapterGetGroupDataMatchesByNameAndPrefersLatest(t *testing.T) {
	initTestStorageBackend(t)

	now := time.Now().UTC()
	saveGroupFixture(t, "group-old", "compute", "old description", "#cloud-config\nold: true\n", now.Add(-time.Minute))
	saveGroupFixture(t, "group-new", "compute", "new description", "#cloud-config\nnew: true\n", now)

	got, err := NewStorageAdapter().GetGroupData("compute")
	if err != nil {
		t.Fatalf("GetGroupData returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected group data, got nil")
		return
	}
	if got.Spec.Description != "new description" {
		t.Fatalf("expected latest group description, got %q", got.Spec.Description)
	}
	if !strings.Contains(got.Spec.Template, "new: true") {
		t.Fatalf("expected latest group template, got %q", got.Spec.Template)
	}
}

func TestRegisterCustomServerIntegrationsAllowsGeneratedCreateRoutes(t *testing.T) {
	initTestStorageBackend(t)
	originalMockSMD := mockSMD
	mockSMD = true
	t.Cleanup(func() { mockSMD = originalMockSMD })
	t.Setenv("SMD_URL", "")
	t.Setenv("SMD_JWT", "")
	t.Setenv("SMD_TOKEN", "")

	r := chi.NewRouter()
	if err := registerCustomServerIntegrations(context.Background(), r); err != nil {
		t.Fatalf("registerCustomServerIntegrations returned error: %v", err)
	}

	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader(`{"metadata":{"name":"test-cluster"},"spec":{"base_url":"http://example.com","cluster_name":"testcluster"}}`)
	resp, err := http.Post(server.URL+"/clusterdefaultss", "application/json", body)
	if err != nil {
		t.Fatalf("POST /clusterdefaultss failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %v", err)
		}
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from generated create route, got %d: %s", resp.StatusCode, string(responseBody))
	}
	if !strings.Contains(string(responseBody), `"uid":"clusterdefaults-`) {
		t.Fatalf("expected generated UID with clusterdefaults prefix, got %s", string(responseBody))
	}
}

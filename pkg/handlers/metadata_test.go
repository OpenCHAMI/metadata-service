// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package handlers_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenCHAMI/cloud-init/pkg/handlers"
	"github.com/OpenCHAMI/cloud-init/pkg/resources/group"
	"github.com/OpenCHAMI/cloud-init/pkg/smdclient"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// mockStore implements the handlers.Store interface for testing
type mockStore struct {
	clusterDefaults *handlers.ClusterDefaults
	instanceInfo    map[string]*handlers.InstanceInfo
	groupData       map[string]*group.Group
}

func (m *mockStore) GetClusterDefaults() (*handlers.ClusterDefaults, error) {
	if m.clusterDefaults == nil {
		return nil, fmt.Errorf("no cluster defaults")
	}
	return m.clusterDefaults, nil
}

func (m *mockStore) GetInstanceInfo(id string) (*handlers.InstanceInfo, error) {
	if info, ok := m.instanceInfo[id]; ok {
		return info, nil
	}
	return nil, fmt.Errorf("no instance info for %s", id)
}

func (m *mockStore) GetGroupData(name string) (*group.Group, error) {
	if data, ok := m.groupData[name]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("no data for group %s", name)
}

func TestMetaDataHandler(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:ff",
		IP:   "10.0.0.100",
	})
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "green"})

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:          "http://test.local",
			CloudProvider:    "OpenCHAMI",
			Region:           "us-test-1",
			AvailabilityZone: "test-az-1",
			ClusterName:      "testcluster",
			ShortName:        "tc",
			NidLength:        4,
			PublicKeys:       []string{"ssh-rsa AAAAB3... default@key"},
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Description: "Compute nodes",
					MetaData: map[string]string{
						"custom_key": "custom_value",
					},
				},
			},
		},
	}

	// Create handler
	handler := handlers.MetaDataHandler(smd, store)

	// Create test request
	req := httptest.NewRequest("GET", "/meta-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	// Call handler
	handler(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Parse response as YAML
	var metadata handlers.MetaData
	if err := yaml.Unmarshal(body, &metadata); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	// Verify metadata fields
	if metadata.InstanceID != "x1000c0s0b0n0" {
		t.Errorf("Expected instance ID 'x1000c0s0b0n0', got '%s'", metadata.InstanceID)
	}

	if metadata.LocalHostname != "tc1000" {
		t.Errorf("Expected local hostname 'tc1000', got '%s'", metadata.LocalHostname)
	}

	if metadata.ClusterName != "testcluster" {
		t.Errorf("Expected cluster name 'testcluster', got '%s'", metadata.ClusterName)
	}

	// Check instance data
	if metadata.InstanceData.V1.CloudProvider != "OpenCHAMI" {
		t.Errorf("Expected cloud provider 'OpenCHAMI', got '%s'", metadata.InstanceData.V1.CloudProvider)
	}

	if metadata.InstanceData.V1.LocalIPv4 != "10.0.0.100" {
		t.Errorf("Expected local IP '10.0.0.100', got '%s'", metadata.InstanceData.V1.LocalIPv4)
	}

	// Check vendor data groups
	if len(metadata.InstanceData.V1.VendorData.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(metadata.InstanceData.V1.VendorData.Groups))
	}

	if computeGroup, ok := metadata.InstanceData.V1.VendorData.Groups["compute"]; !ok {
		t.Error("Expected 'compute' group in vendor data")
	} else {
		if desc, ok := computeGroup["description"].(string); !ok || desc != "Compute nodes" {
			t.Errorf("Expected compute group description 'Compute nodes', got '%v'", computeGroup["description"])
		}
	}
}

func TestMetaDataHandler_XForwardedFor(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			ClusterName: "testcluster",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData:    map[string]*group.Group{},
	}

	// Create handler
	handler := handlers.MetaDataHandler(smd, store)

	// Create test request with X-Forwarded-For header
	req := httptest.NewRequest("GET", "/meta-data", nil)
	req.RemoteAddr = "192.168.1.1:54321" // Proxy IP
	req.Header.Set("X-Forwarded-For", "10.0.0.100, 192.168.1.1")
	w := httptest.NewRecorder()

	// Call handler
	handler(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Parse response as YAML
	var metadata handlers.MetaData
	if err := yaml.Unmarshal(body, &metadata); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	// Verify it resolved to the correct component
	if metadata.InstanceID != "x1000c0s0b0n0" {
		t.Errorf("Expected instance ID 'x1000c0s0b0n0', got '%s'", metadata.InstanceID)
	}
}

func TestMetaDataHandler_UnknownIP(t *testing.T) {
	// Setup mock SMD client (empty)
	smd := smdclient.NewMockSMDClient()

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{},
		instanceInfo:    map[string]*handlers.InstanceInfo{},
		groupData:       map[string]*group.Group{},
	}

	// Create handler
	handler := handlers.MetaDataHandler(smd, store)

	// Create test request from unknown IP
	req := httptest.NewRequest("GET", "/meta-data", nil)
	req.RemoteAddr = "192.168.99.99:12345"
	w := httptest.NewRecorder()

	// Call handler
	handler(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestUserDataHandler(t *testing.T) {
	handler := http.HandlerFunc(handlers.UserDataHandler)

	req := httptest.NewRequest("GET", "/user-data", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expected := "#cloud-config\n"
	if string(body) != expected {
		t.Errorf("Expected body '%s', got '%s'", expected, string(body))
	}
}

func TestVendorDataHandler(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "green"})

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL: "http://test.local",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData:    map[string]*group.Group{},
	}

	// Create handler
	handler := handlers.VendorDataHandler(smd, store)

	// Create test request
	req := httptest.NewRequest("GET", "/vendor-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	// Call handler
	handler(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expected := "#include\nhttp://test.local/compute.yaml\nhttp://test.local/green.yaml\n"
	if string(body) != expected {
		t.Errorf("Expected body:\n%s\nGot:\n%s", expected, string(body))
	}
}

func TestGroupUserDataHandler(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	// Setup mock store with group template
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			ClusterName: "testcluster",
			ShortName:   "tc",
			NidLength:   4,
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Description: "Compute nodes",
					Template: `#cloud-config
hostname: {{ hostname }}
fqdn: {{ hostname }}.testcluster.local
`,
					MetaData: map[string]string{},
				},
			},
		},
	}

	// Create handler
	handler := handlers.GroupUserDataHandler(smd, store)

	// Setup router with group parameter
	r := chi.NewRouter()
	r.Get("/{group}.yaml", handler)

	// Create test request
	req := httptest.NewRequest("GET", "/compute.yaml", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	// Call handler
	r.ServeHTTP(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expected := `#cloud-config
hostname: tc1000
fqdn: tc1000.testcluster.local
`
	if string(body) != expected {
		t.Errorf("Expected body:\n%s\nGot:\n%s", expected, string(body))
	}
}

func TestGroupUserDataHandler_NotMember(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{},
		instanceInfo:    map[string]*handlers.InstanceInfo{},
		groupData:       map[string]*group.Group{},
	}

	// Create handler
	handler := handlers.GroupUserDataHandler(smd, store)

	// Setup router with group parameter
	r := chi.NewRouter()
	r.Get("/{group}.yaml", handler)

	// Create test request for group node is NOT a member of
	req := httptest.NewRequest("GET", "/storage.yaml", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	// Call handler
	r.ServeHTTP(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", resp.StatusCode)
	}
}

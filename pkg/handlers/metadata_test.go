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
					Template:    "#cloud-config\npackages:\n  - git\n",
					MetaData: map[string]string{
						"custom_key": "custom_value",
					},
				},
			},
			"green": {
				Spec: group.GroupSpec{
					Description: "Green nodes",
					Template:    "#cloud-config\nruncmd:\n  - echo green\n",
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

func TestMetaDataHandler_WithInterfaces(t *testing.T) {
	// Setup mock SMD client with EthernetInterface data
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "b4:2e:99:be:1a:6d",
		IP:   "10.252.0.26",
	})

	// Add EthernetNICInfo (2 NICs)
	smd.AddEthernetNICInfo("x1000c0s0b0n0", []smdclient.EthernetNIC{
		{
			RedfishID:           "1",
			Description:         "Node Management Network",
			MACAddress:          "b4:2e:99:be:1a:6d",
			PermanentMACAddress: "b4:2e:99:be:1a:6d",
			InterfaceEnabled:    true,
		},
		{
			RedfishID:           "2",
			Description:         "High Speed Network",
			MACAddress:          "b4:2e:99:be:1a:6e",
			PermanentMACAddress: "b4:2e:99:be:1a:6e",
			InterfaceEnabled:    true,
		},
	})

	// Add EthernetInterfaces (IP/Network mappings)
	smd.AddEthernetInterfaces("x1000c0s0b0n0", []smdclient.EthernetInterface{
		{
			ID:          "b42e99be1a6d",
			Description: "Node Management Network",
			MACAddress:  "b4:2e:99:be:1a:6d",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.252.0.26", Network: "HMN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a6e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:6e",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.100.0.26", Network: "HSN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
	})

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			ClusterName: "testcluster",
			ShortName:   "tc",
			NidLength:   4,
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData:    map[string]*group.Group{},
	}

	// Create handler
	handler := handlers.MetaDataHandler(smd, store)

	// Create test request
	req := httptest.NewRequest("GET", "/meta-data", nil)
	req.RemoteAddr = "10.252.0.26:12345"
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

	// Verify interfaces are included in vendor data
	if len(metadata.InstanceData.V1.VendorData.Interfaces) != 2 {
		t.Fatalf("Expected 2 interfaces in vendor data, got %d", len(metadata.InstanceData.V1.VendorData.Interfaces))
	}

	// Verify first interface
	iface0 := metadata.InstanceData.V1.VendorData.Interfaces[0]
	if iface0["name"] != "eth0" {
		t.Errorf("Expected interface name 'eth0', got '%v'", iface0["name"])
	}
	if iface0["mac"] != "b4:2e:99:be:1a:6d" {
		t.Errorf("Expected first MAC 'b4:2e:99:be:1a:6d', got '%v'", iface0["mac"])
	}
	if iface0["ip"] != "10.252.0.26" {
		t.Errorf("Expected first IP '10.252.0.26', got '%v'", iface0["ip"])
	}
	if iface0["network"] != "HMN" {
		t.Errorf("Expected first network 'HMN', got '%v'", iface0["network"])
	}

	// Verify second interface
	iface1 := metadata.InstanceData.V1.VendorData.Interfaces[1]
	if iface1["name"] != "eth1" {
		t.Errorf("Expected interface name 'eth1', got '%v'", iface1["name"])
	}
	if iface1["mac"] != "b4:2e:99:be:1a:6e" {
		t.Errorf("Expected second MAC 'b4:2e:99:be:1a:6e', got '%v'", iface1["mac"])
	}
	if iface1["ip"] != "10.100.0.26" {
		t.Errorf("Expected second IP '10.100.0.26', got '%v'", iface1["ip"])
	}
	if iface1["network"] != "HSN" {
		t.Errorf("Expected second network 'HSN', got '%v'", iface1["network"])
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
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Template: "#cloud-config\npackages: []\n",
				},
			},
			"green": {
				Spec: group.GroupSpec{
					Template: "#cloud-config\nruncmd: []\n",
				},
			},
		},
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

// TestMetaDataHandler_Issue100_FilterEmptyGroups verifies that groups with empty templates
// are excluded from vendor_data.Groups to prevent issue #100 (empty cloud-config MIME parts)
func TestMetaDataHandler_Issue100_FilterEmptyGroups(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})

	// Setup mock store with groups - some empty, some with content
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:       "http://localhost:8888",
			ClusterName:   "testcluster",
			CloudProvider: "OpenCHAMI",
			Region:        "us-west-1",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Template:    "#cloud-config\npackages:\n  - git\n",
					Description: "Compute nodes",
				},
			},
			"empty-group": {
				Spec: group.GroupSpec{
					Template:    "", // Empty template
					Description: "Empty group",
				},
			},
			"storage": {
				Spec: group.GroupSpec{
					Template:    "#cloud-config\nvolume_groups: []\n",
					Description: "Storage nodes",
				},
			},
		},
	}

	// Add node to multiple groups including empty one
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "empty-group", "storage"})

	// Create handler and request
	handler := handlers.MetaDataHandler(smd, store)
	req := httptest.NewRequest("GET", "/meta-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	// Execute
	handler(w, req)

	// Verify response
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Parse YAML response
	var metadata handlers.MetaData
	if err := yaml.Unmarshal(body, &metadata); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	// Verify empty groups are NOT included in vendor_data
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["empty-group"]; ok {
		t.Error("Empty group should not be in vendor_data")
	}

	// Verify non-empty groups ARE included
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["compute"]; !ok {
		t.Error("Compute group should be in vendor_data")
	}
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["storage"]; !ok {
		t.Error("Storage group should be in vendor_data")
	}
}

// TestVendorDataHandler_Issue100_FilterEmptyGroups verifies that groups with empty templates
// are excluded from vendor-data include list to prevent issue #100
func TestVendorDataHandler_Issue100_FilterEmptyGroups(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})

	// Setup mock store with groups
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL: "http://localhost:8888",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Template: "#cloud-config\npackages: [git]\n",
				},
			},
			"empty-group": {
				Spec: group.GroupSpec{
					Template: "", // Empty
				},
			},
			"storage": {
				Spec: group.GroupSpec{
					Template: "#cloud-config\nvolume_groups: []\n",
				},
			},
		},
	}

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "empty-group", "storage"})

	handler := handlers.VendorDataHandler(smd, store)
	req := httptest.NewRequest("GET", "/vendor-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	response := string(body)

	// Verify format
	if !strings.HasPrefix(response, "#include\n") {
		t.Error("Response should start with #include")
	}

	// Verify empty group is NOT in include list
	if strings.Contains(response, "empty-group.yaml") {
		t.Error("Empty group should not be in vendor-data include list")
	}

	// Verify non-empty groups ARE in include list
	if !strings.Contains(response, "compute.yaml") {
		t.Error("Compute group should be in vendor-data include list")
	}
	if !strings.Contains(response, "storage.yaml") {
		t.Error("Storage group should be in vendor-data include list")
	}
}

// TestVendorDataHandler_Issue100_AllGroupsEmpty verifies behavior when all groups are empty
func TestVendorDataHandler_Issue100_AllGroupsEmpty(t *testing.T) {
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
			BaseURL: "http://localhost:8888",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"empty1": {
				Spec: group.GroupSpec{
					Template: "",
				},
			},
			"empty2": {
				Spec: group.GroupSpec{
					Template: "",
				},
			},
		},
	}

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"empty1", "empty2"})

	handler := handlers.VendorDataHandler(smd, store)
	req := httptest.NewRequest("GET", "/vendor-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	response := strings.TrimSpace(string(body))

	// Should only have header, no group includes
	lines := strings.Split(response, "\n")
	if len(lines) != 1 {
		t.Errorf("Should only have #include header, got %d lines", len(lines))
	}
	if lines[0] != "#include" {
		t.Errorf("Expected '#include', got '%s'", lines[0])
	}
}

// TestMetaDataHandler_Issue100_MissingGroupData verifies that groups with missing data
// are excluded from vendor_data.Groups
func TestMetaDataHandler_Issue100_MissingGroupData(t *testing.T) {
	// Setup mock SMD client
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		IP:   "10.0.0.100",
	})

	// Setup mock store - compute group exists, missing-group does not
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL: "http://localhost:8888",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Template: "#cloud-config\npackages: []\n",
				},
			},
			// "missing-group" intentionally not in map
		},
	}

	// Add node to both groups
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "missing-group"})

	handler := handlers.MetaDataHandler(smd, store)
	req := httptest.NewRequest("GET", "/meta-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var metadata handlers.MetaData
	if err := yaml.Unmarshal(body, &metadata); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	// Verify missing group is NOT included
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["missing-group"]; ok {
		t.Error("Missing group should not be in vendor_data")
	}
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["compute"]; !ok {
		t.Error("Existing group should be in vendor_data")
	}
}

// TestVendorDataHandler_Issue100_MissingGroupData verifies that groups with missing data
// are excluded from vendor-data include list
func TestVendorDataHandler_Issue100_MissingGroupData(t *testing.T) {
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
			BaseURL: "http://localhost:8888",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"compute": {
				Spec: group.GroupSpec{
					Template: "#cloud-config\npackages: []\n",
				},
			},
			// "missing-group" not in map
		},
	}

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "missing-group"})

	handler := handlers.VendorDataHandler(smd, store)
	req := httptest.NewRequest("GET", "/vendor-data", nil)
	req.RemoteAddr = "10.0.0.100:12345"
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	response := string(body)

	// Verify missing group is NOT in include list
	if strings.Contains(response, "missing-group.yaml") {
		t.Error("Missing group should not be in vendor-data include list")
	}
	if !strings.Contains(response, "compute.yaml") {
		t.Error("Existing group should be in vendor-data include list")
	}
}

// TestGroupUserDataHandler_WithNetworkConfig verifies that EthernetInterface data
// is injected into group templates as the interfaces array
func TestGroupUserDataHandler_WithNetworkConfig(t *testing.T) {
	// Setup mock SMD client with EthernetInterface data
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "b4:2e:99:be:1a:6d",
		IP:   "10.252.0.26",
	})

	// Add EthernetNICInfo (2 NICs)
	smd.AddEthernetNICInfo("x1000c0s0b0n0", []smdclient.EthernetNIC{
		{
			RedfishID:           "1",
			Description:         "Node Management Network",
			MACAddress:          "b4:2e:99:be:1a:6d",
			PermanentMACAddress: "b4:2e:99:be:1a:6d",
			InterfaceEnabled:    true,
		},
		{
			RedfishID:           "2",
			Description:         "High Speed Network",
			MACAddress:          "b4:2e:99:be:1a:6e",
			PermanentMACAddress: "b4:2e:99:be:1a:6e",
			InterfaceEnabled:    true,
		},
	})

	// Add EthernetInterfaces (IP/Network mappings)
	smd.AddEthernetInterfaces("x1000c0s0b0n0", []smdclient.EthernetInterface{
		{
			ID:          "b42e99be1a6d",
			Description: "Node Management Network",
			MACAddress:  "b4:2e:99:be:1a:6d",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.252.0.26", Network: "HMN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a6e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:6e",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.100.0.26", Network: "HSN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
	})

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"network-config"})

	// Setup mock store with network config template
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:       "http://localhost:8888",
			CloudProvider: "OpenCHAMI",
			Region:        "us-west-1",
			ClusterName:   "testcluster",
			ShortName:     "tc",
			NidLength:     4,
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*group.Group{
			"network-config": {
				Spec: group.GroupSpec{
					Description: "Network configuration for nodes",
					Template: `#cloud-config
network:
  version: 1
  config:
{% for iface in interfaces %}    - type: physical
      name: {{ iface.name }}
      mac_address: {{ iface.mac }}
      subnets:
        - type: static
          address: {{ iface.ip }}/24
{% endfor %}`,
					MetaData: map[string]string{},
				},
			},
		},
	}

	handler := handlers.GroupUserDataHandler(smd, store)

	// Setup router with group parameter
	r := chi.NewRouter()
	r.Get("/{group}.yaml", handler)

	// Create test request
	req := httptest.NewRequest("GET", "/network-config.yaml", nil)
	req.RemoteAddr = "10.252.0.26:12345"
	w := httptest.NewRecorder()

	// Call handler via router
	r.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	rendered := string(body)

	// Verify rendered output contains both interfaces with correct data
	if !strings.Contains(rendered, "eth0") {
		t.Error("Expected eth0 in rendered output")
	}
	if !strings.Contains(rendered, "eth1") {
		t.Error("Expected eth1 in rendered output")
	}
	if !strings.Contains(rendered, "b4:2e:99:be:1a:6d") {
		t.Error("Expected first MAC address in rendered output")
	}
	if !strings.Contains(rendered, "b4:2e:99:be:1a:6e") {
		t.Error("Expected second MAC address in rendered output")
	}
	if !strings.Contains(rendered, "10.252.0.26") {
		t.Error("Expected first IP address in rendered output")
	}
	if !strings.Contains(rendered, "10.100.0.26") {
		t.Error("Expected second IP address in rendered output")
	}
}

// TestNetworkConfigHandler verifies the /network-config endpoint
// returns cloud-init network config v1 format with SMD data
func TestNetworkConfigHandler(t *testing.T) {
	// Setup mock SMD client with EthernetInterface data
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "b4:2e:99:be:1a:6d",
		IP:   "10.252.0.26",
	})

	// Add EthernetNICInfo (2 NICs)
	smd.AddEthernetNICInfo("x1000c0s0b0n0", []smdclient.EthernetNIC{
		{
			RedfishID:           "1",
			Description:         "Node Management Network",
			MACAddress:          "b4:2e:99:be:1a:6d",
			PermanentMACAddress: "b4:2e:99:be:1a:6d",
			InterfaceEnabled:    true,
		},
		{
			RedfishID:           "2",
			Description:         "High Speed Network",
			MACAddress:          "b4:2e:99:be:1a:6e",
			PermanentMACAddress: "b4:2e:99:be:1a:6e",
			InterfaceEnabled:    true,
		},
	})

	// Add EthernetInterfaces (IP/Network mappings)
	smd.AddEthernetInterfaces("x1000c0s0b0n0", []smdclient.EthernetInterface{
		{
			ID:          "b42e99be1a6d",
			Description: "Node Management Network",
			MACAddress:  "b4:2e:99:be:1a:6d",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.252.0.26", Network: "HMN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a6e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:6e",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.100.0.26", Network: "HSN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
	})

	// Setup mock store
	store := &mockStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:       "http://localhost:8888",
			CloudProvider: "OpenCHAMI",
			Region:        "us-west-1",
			ClusterName:   "testcluster",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData:    map[string]*group.Group{},
	}

	handler := handlers.NetworkConfigHandler(smd, store)

	req := httptest.NewRequest("GET", "/network-config", nil)
	req.RemoteAddr = "10.252.0.26:12345"
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	rendered := string(body)

	// Verify network-config structure
	if !strings.Contains(rendered, "version: 1") {
		t.Error("Expected network-config version 1 in output")
	}
	if !strings.Contains(rendered, "config:") {
		t.Error("Expected 'config' section in network-config")
	}
	if !strings.Contains(rendered, "type: physical") {
		t.Error("Expected physical interface type in network-config")
	}

	// Verify interfaces are present
	if !strings.Contains(rendered, "eth0") {
		t.Error("Expected eth0 in network-config")
	}
	if !strings.Contains(rendered, "eth1") {
		t.Error("Expected eth1 in network-config")
	}

	// Verify MAC addresses
	if !strings.Contains(rendered, "b4:2e:99:be:1a:6d") {
		t.Error("Expected first MAC address in network-config")
	}
	if !strings.Contains(rendered, "b4:2e:99:be:1a:6e") {
		t.Error("Expected second MAC address in network-config")
	}

	// Verify IP addresses
	if !strings.Contains(rendered, "10.252.0.26") {
		t.Error("Expected first IP address in network-config")
	}
	if !strings.Contains(rendered, "10.100.0.26") {
		t.Error("Expected second IP address in network-config")
	}

	// Verify subnet format
	if !strings.Contains(rendered, "subnets:") {
		t.Error("Expected subnets section in network-config")
	}
	if !strings.Contains(rendered, "address:") {
		t.Error("Expected address in subnets")
	}
}

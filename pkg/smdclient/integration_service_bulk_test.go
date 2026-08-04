// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// failingBulkBackend is a mock that fails bulk operations
type failingBulkBackend struct {
	*MockSMDClient
	failGroups     bool
	failInterfaces bool
	failNICs       bool
}

func (f *failingBulkBackend) BulkGroupMemberships() (map[string][]string, error) {
	if f.failGroups {
		return nil, fmt.Errorf("forced bulk groups failure")
	}
	return f.MockSMDClient.BulkGroupMemberships()
}

func (f *failingBulkBackend) BulkEthernetInterfaces() (map[string][]EthernetInterface, error) {
	if f.failInterfaces {
		return nil, fmt.Errorf("forced bulk interfaces failure")
	}
	return f.MockSMDClient.BulkEthernetInterfaces()
}

func (f *failingBulkBackend) BulkEthernetNICInfo() (map[string][]EthernetNIC, error) {
	if f.failNICs {
		return nil, fmt.Errorf("forced bulk NICs failure")
	}
	return f.MockSMDClient.BulkEthernetNICInfo()
}

// minimalBackend is a minimal backend that only implements SMDClient and ComponentLister
// but does NOT implement BulkDataFetcher (for testing per-component fallback)
type minimalBackend struct {
	components map[string]*Component
	groups     map[string][]string
	ifaces     map[string][]EthernetInterface
	nics       map[string][]EthernetNIC
}

func (m *minimalBackend) IDfromIP(ip string) (string, error)                        { return "", fmt.Errorf("not implemented") }
func (m *minimalBackend) IDfromWGIP(wgip string) (string, error)                    { return "", fmt.Errorf("not implemented") }
func (m *minimalBackend) IPfromID(id string) (string, error)                        { return "", fmt.Errorf("not implemented") }
func (m *minimalBackend) MACfromID(id string) (string, error)                       { return "", fmt.Errorf("not implemented") }
func (m *minimalBackend) ComponentInformation(id string) (*Component, error)        { return m.components[id], nil }
func (m *minimalBackend) GroupMembership(id string) ([]string, error)               { return m.groups[id], nil }
func (m *minimalBackend) AddWGIP(id, wgip string) error                             { return nil }
func (m *minimalBackend) WGIPfromID(id string) (string, error)                      { return "", fmt.Errorf("not found") }
func (m *minimalBackend) EthernetNICInfo(id string) ([]EthernetNIC, error)          { return m.nics[id], nil }
func (m *minimalBackend) EthernetInterfaces(id string) ([]EthernetInterface, error) { return m.ifaces[id], nil }
func (m *minimalBackend) ListComponents() ([]*Component, error) {
	result := make([]*Component, 0, len(m.components))
	for _, comp := range m.components {
		result = append(result, comp)
	}
	return result, nil
}

func TestSMDIntegrationServiceBulkSyncSuccess(t *testing.T) {
	backend := NewMockSMDClient()
	
	// Add test components
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	backend.AddComponent(&Component{ID: "x1000c0s0b0n1", NID: 1001, Role: "compute", IP: "10.0.0.101"})
	
	// Add group memberships
	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "ntp"})
	backend.AddGroupMembership("x1000c0s0b0n1", []string{"compute"})
	
	// Add ethernet interfaces
	backend.AddEthernetInterfaces("x1000c0s0b0n0", []EthernetInterface{
		{
			ID:          "0040a6835ba0",
			MACAddress:  "00:40:a6:83:5b:a0",
			IPAddresses: []IPMapping{{IPAddress: "10.0.0.100", Network: "HMN"}},
			ComponentID: "x1000c0s0b0n0",
		},
	})
	backend.AddEthernetInterfaces("x1000c0s0b0n1", []EthernetInterface{
		{
			ID:          "0040a6835ba1",
			MACAddress:  "00:40:a6:83:5b:a1",
			IPAddresses: []IPMapping{{IPAddress: "10.0.0.101", Network: "HMN"}},
			ComponentID: "x1000c0s0b0n1",
		},
	})
	
	// Add ethernet NICs
	backend.AddEthernetNICInfo("x1000c0s0b0n0", []EthernetNIC{
		{RedfishID: "1", MACAddress: "00:40:a6:83:5b:a0", InterfaceEnabled: true},
	})
	backend.AddEthernetNICInfo("x1000c0s0b0n1", []EthernetNIC{
		{RedfishID: "1", MACAddress: "00:40:a6:83:5b:a1", InterfaceEnabled: true},
	})
	
	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	
	// Verify bulk fetcher is available
	if service.bulkFetcher == nil {
		t.Fatal("bulk fetcher should be available for MockSMDClient")
	}
	
	// Perform sync
	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}
	
	// Verify first component
	groups, err := service.GroupMembership("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("GroupMembership failed: %v", err)
	}
	if len(groups) != 2 || groups[0] != "compute" || groups[1] != "ntp" {
		t.Errorf("expected groups [compute, ntp], got %v", groups)
	}
	
	ifaces, err := service.EthernetInterfaces("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("EthernetInterfaces failed: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0].MACAddress != "00:40:a6:83:5b:a0" {
		t.Errorf("expected 1 interface with MAC 00:40:a6:83:5b:a0, got %v", ifaces)
	}
	
	nics, err := service.EthernetNICInfo("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("EthernetNICInfo failed: %v", err)
	}
	if len(nics) != 1 || nics[0].RedfishID != "1" {
		t.Errorf("expected 1 NIC with RedfishID 1, got %v", nics)
	}
	
	// Verify second component
	groups, err = service.GroupMembership("x1000c0s0b0n1")
	if err != nil {
		t.Fatalf("GroupMembership failed for second component: %v", err)
	}
	if len(groups) != 1 || groups[0] != "compute" {
		t.Errorf("expected groups [compute], got %v", groups)
	}
	
	// Verify IP resolution
	id, err := service.ResolveComponentID("10.0.0.101")
	if err != nil {
		t.Fatalf("ResolveComponentID failed: %v", err)
	}
	if id != "x1000c0s0b0n1" {
		t.Errorf("expected x1000c0s0b0n1, got %s", id)
	}
}

func TestSMDIntegrationServiceBulkSyncFailureRetainsCache(t *testing.T) {
	mock := NewMockSMDClient()
	mock.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	mock.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})
	
	backend := &failingBulkBackend{
		MockSMDClient: mock,
		failGroups:    false,
	}
	
	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	
	// First sync should succeed
	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("initial syncOnce failed: %v", err)
	}
	
	// Verify cache is populated
	groups, err := service.GroupMembership("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("GroupMembership failed: %v", err)
	}
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
	
	// Now make bulk operations fail
	backend.failGroups = true
	
	// Second sync should fail but retain cache
	err = service.syncOnce(context.Background())
	if err == nil {
		t.Fatal("expected syncOnce to fail when bulk operations fail")
	}
	
	// Verify cache is still valid (stale but available)
	service.mu.RLock()
	cacheSize := len(service.nodes)
	service.mu.RUnlock()
	
	if cacheSize != 1 {
		t.Errorf("expected cache to retain 1 component, got %d", cacheSize)
	}
	
	// Should still be able to get groups from stale cache
	groups, err = service.GroupMembership("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("GroupMembership failed after sync failure: %v", err)
	}
	if len(groups) != 1 {
		t.Errorf("expected stale cache to still have 1 group, got %d", len(groups))
	}
}

func TestSMDIntegrationServiceBulkSyncPartialFailure(t *testing.T) {
	mock := NewMockSMDClient()
	mock.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	mock.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})
	mock.AddEthernetInterfaces("x1000c0s0b0n0", []EthernetInterface{
		{ID: "test", MACAddress: "00:40:a6:83:5b:a0", ComponentID: "x1000c0s0b0n0"},
	})
	
	backend := &failingBulkBackend{
		MockSMDClient:  mock,
		failInterfaces: true, // Fail interfaces lookup
	}
	
	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	
	// Sync should fail because interfaces fetch fails
	err := service.syncOnce(context.Background())
	if err == nil {
		t.Fatal("expected syncOnce to fail when BulkEthernetInterfaces fails")
	}
	
	// Cache should be empty (not partially populated)
	service.mu.RLock()
	cacheSize := len(service.nodes)
	service.mu.RUnlock()
	
	if cacheSize != 0 {
		t.Errorf("expected empty cache after partial failure, got %d components", cacheSize)
	}
}

func TestSMDIntegrationServiceBulkSyncContextCancellation(t *testing.T) {
	backend := NewMockSMDClient()
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	
	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	
	// Sync should fail with context error
	err := service.syncOnce(ctx)
	if err == nil {
		t.Fatal("expected syncOnce to fail with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestSMDIntegrationServicePerComponentFallbackWhenNoBulkFetcher(t *testing.T) {
	// Create a minimal backend that only implements SMDClient and ComponentLister
	backend := &minimalBackend{
		components: map[string]*Component{
			"x1000c0s0b0n0": {ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"},
		},
		groups: map[string][]string{
			"x1000c0s0b0n0": {"compute"},
		},
		ifaces: make(map[string][]EthernetInterface),
		nics:   make(map[string][]EthernetNIC),
	}
	
	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	
	// Verify bulk fetcher is NOT available
	if service.bulkFetcher != nil {
		t.Fatal("bulk fetcher should NOT be available for minimalBackend")
	}
	
	// Sync should still work using per-component fallback
	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce with per-component fallback failed: %v", err)
	}
	
	// Verify data is populated
	groups, err := service.GroupMembership("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("GroupMembership failed: %v", err)
	}
	if len(groups) != 1 || groups[0] != "compute" {
		t.Errorf("expected groups [compute], got %v", groups)
	}
}


// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type controlledBackend struct {
	*MockSMDClient

	failComponent bool
}

func (c *controlledBackend) ComponentInformation(id string) (*Component, error) {
	if c.failComponent {
		return nil, fmt.Errorf("forced component error")
	}
	return c.MockSMDClient.ComponentInformation(id)
}

func waitFor(t *testing.T, timeout time.Duration, check func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition: %s", message)
}

func TestSMDIntegrationServiceSyncWorkerInitialAndPeriodic(t *testing.T) {
	backend := NewMockSMDClient()
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: 40 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service.StartSyncWorker(ctx)

	waitFor(t, time.Second, func() bool {
		groups, err := service.GroupMembership("x1000c0s0b0n0")
		return err == nil && len(groups) == 1 && groups[0] == "compute"
	}, "initial sync should populate groups")

	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "ntp"})
	waitFor(t, time.Second, func() bool {
		groups, err := service.GroupMembership("x1000c0s0b0n0")
		if err != nil || len(groups) != 2 {
			return false
		}
		return groups[0] == "compute" && groups[1] == "ntp"
	}, "periodic sync should refresh group membership")
}

func TestSMDIntegrationServiceAddWGIPUpdatesResolver(t *testing.T) {
	backend := NewMockSMDClient()
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}

	if err := service.AddWGIP("x1000c0s0b0n0", "10.100.1.25"); err != nil {
		t.Fatalf("AddWGIP failed: %v", err)
	}

	id, err := service.ResolveComponentID("10.100.1.25")
	if err != nil {
		t.Fatalf("ResolveComponentID failed: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected x1000c0s0b0n0, got %q", id)
	}
}

func TestSMDIntegrationServiceStaleCacheFallsBackToLiveThenCache(t *testing.T) {
	mock := NewMockSMDClient()
	mock.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	mock.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})
	backend := &controlledBackend{MockSMDClient: mock}

	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: 100 * time.Millisecond})
	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}

	service.mu.Lock()
	service.lastRun = time.Now().Add(-time.Second)
	service.mu.Unlock()

	backend.failComponent = true
	component, err := service.ComponentInformation("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("expected stale cache fallback, got error: %v", err)
	}
	if component == nil || component.ID != "x1000c0s0b0n0" {
		t.Fatalf("unexpected component %+v", component)
	}
}

func TestSMDIntegrationServiceResolvePrecedenceWireGuardFirst(t *testing.T) {
	backend := NewMockSMDClient()
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	if err := backend.AddWGIP("x1000c0s0b0n0", "10.100.1.25"); err != nil {
		t.Fatalf("backend AddWGIP failed: %v", err)
	}

	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}

	id, err := service.ResolveComponentID("10.100.1.25")
	if err != nil {
		t.Fatalf("ResolveComponentID failed: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected x1000c0s0b0n0, got %q", id)
	}
}

func TestSMDIntegrationServiceInitialSyncStatus(t *testing.T) {
	backend := NewMockSMDClient()
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})

	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	if healthy, reason := service.InitialSyncStatus(); healthy || reason != "smd initial refresh pending" {
		t.Fatalf("expected pending initial sync status, got healthy=%v reason=%q", healthy, reason)
	}

	if err := service.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}

	if healthy, reason := service.InitialSyncStatus(); !healthy || reason != "" {
		t.Fatalf("expected healthy status after initial sync, got healthy=%v reason=%q", healthy, reason)
	}
}

type countingBackend struct {
	*MockSMDClient

	countMu sync.Mutex
	count   int
}

func (c *countingBackend) ListComponents() ([]*Component, error) {
	c.countMu.Lock()
	c.count++
	c.countMu.Unlock()
	return c.MockSMDClient.ListComponents()
}

func (c *countingBackend) Calls() int {
	c.countMu.Lock()
	defer c.countMu.Unlock()
	return c.count
}

func TestSMDIntegrationServiceSyncWorkerStopsOnCancel(t *testing.T) {
	backend := &countingBackend{MockSMDClient: NewMockSMDClient()}
	backend.AddComponent(&Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	service := NewSMDIntegrationService(backend, IntegrationOptions{SyncEnabled: true, SyncInterval: 30 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	service.StartSyncWorker(ctx)

	waitFor(t, time.Second, func() bool { return backend.Calls() > 0 }, "worker should execute list calls")
	cancel()

	before := backend.Calls()
	time.Sleep(120 * time.Millisecond)
	after := backend.Calls()
	if after > before+1 {
		t.Fatalf("expected worker to stop on cancel, calls before=%d after=%d", before, after)
	}
}

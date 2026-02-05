package reconcilers

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

import (
	"context"
	"encoding/json"
	"testing"

	v1 "github.com/OpenCHAMI/cloud-init/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/cloud-init/internal/storage"
	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
	"github.com/openchami/fabrica/pkg/events"
)

type mockDevice struct {
	addCalls []addCall
}

type addCall struct {
	publicKey string
	allowedIP string
}

func (m *mockDevice) SetPrivateKey(string) error { return nil }
func (m *mockDevice) AddPeer(publicKey, allowedIP string) error {
	m.addCalls = append(m.addCalls, addCall{publicKey: publicKey, allowedIP: allowedIP})
	return nil
}
func (m *mockDevice) RemovePeer(string) error { return nil }
func (m *mockDevice) Close() error            { return nil }
func (m *mockDevice) PublicKeyValue() string  { return "" }
func (m *mockDevice) SetPublicKeyValue(string) {
}
func (m *mockDevice) ListenPortValue() int             { return 0 }
func (m *mockDevice) PrivateKeyValue() (string, error) { return "", nil }

func TestWireGuardPeerReconcileUpsert(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()

	if err := storage.InitFileBackend(dataDir); err != nil {
		t.Fatalf("init storage: %v", err)
	}

	peer := &v1.WireGuardPeer{
		APIVersion: "cloud-init.openchami.io/v1",
		Kind:       "WireGuardPeer",
		Spec: v1.WireGuardPeerSpec{
			PublicKey: "pubkey",
			AllowedIP: "10.0.0.2/32",
		},
	}
	peer.Metadata.Initialize("peer-1", "wgp-1")

	if err := storage.SaveWireGuardPeer(ctx, peer); err != nil {
		t.Fatalf("save peer: %v", err)
	}

	device := &mockDevice{}
	controller := &wireguard.Controller{
		Device: device,
		Peers:  map[string]wireguard.PeerState{},
	}
	SetWireGuardController(controller)
	t.Cleanup(func() { SetWireGuardController(nil) })

	eventBus := events.NewInMemoryEventBus(10, 1)
	eventBus.Start()
	t.Cleanup(func() { _ = eventBus.Close() })

	reconciler := NewDefaultWireGuardPeerReconciler(storage.NewStorageClient(), eventBus)

	raw, err := storage.Backend.Load(ctx, "WireGuardPeer", "wgp-1")
	if err != nil {
		t.Fatalf("load raw: %v", err)
	}

	result, err := reconciler.Reconcile(ctx, json.RawMessage(raw))
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue after duration")
	}
	if len(device.addCalls) != 1 {
		t.Fatalf("expected 1 AddPeer call, got %d", len(device.addCalls))
	}
	call := device.addCalls[0]
	if call.publicKey != peer.Spec.PublicKey || call.allowedIP != peer.Spec.AllowedIP {
		t.Fatalf("unexpected AddPeer call: %+v", call)
	}

	updated, err := storage.LoadWireGuardPeer(ctx, "wgp-1")
	if err != nil {
		t.Fatalf("load updated: %v", err)
	}
	if !updated.Status.Ready {
		t.Fatalf("expected status ready")
	}
	if updated.Status.Phase != "Configured" {
		t.Fatalf("expected phase Configured, got %q", updated.Status.Phase)
	}
}

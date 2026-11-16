// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package reconcilers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openchami/fabrica/pkg/events"
	"github.com/openchami/metadata-service/pkg/notifications"
)

type shutdownEventBus struct{ events []events.Event }

func (*shutdownEventBus) Start()       {}
func (*shutdownEventBus) Close() error { return nil }
func (*shutdownEventBus) Subscribe(string, events.EventHandler) (events.SubscriptionID, error) {
	return "", nil
}
func (*shutdownEventBus) Unsubscribe(events.SubscriptionID) error { return nil }
func (b *shutdownEventBus) Publish(_ context.Context, event events.Event) error {
	b.events = append(b.events, event)
	return nil
}

func TestWireGuardShutdownReportsActualOutcome(t *testing.T) {
	tests := []struct {
		name, outcome string
		controller    *mockWireGuardController
		wantError     bool
	}{
		{"removed", "removed", &mockWireGuardController{}, false},
		{"missing", "already_absent", &mockWireGuardController{removeErr: errors.New("peer not found: peer1")}, false},
		{"failure", "failed", &mockWireGuardController{removeErr: errors.New("private diagnostic not for event payload")}, true},
		{"disabled", "skipped_controller_disabled", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &shutdownEventBus{}
			previousBus := events.GetGlobalEventBus()
			events.SetGlobalEventBus(bus)
			defer events.SetGlobalEventBus(previousBus)
			reconciler := NewWireGuardPeerReconciler(newMockReconcileClient(), tt.controller)
			err := reconciler.Delete(context.Background(), "peer1")
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(bus.events) != 1 {
				t.Fatalf("got %d events", len(bus.events))
			}
			event := bus.events[0]
			if event.Type() != notifications.WireGuardShutdown {
				t.Fatalf("type=%s", event.Type())
			}
			var data notifications.Shutdown
			if err := event.DataAs(&data); err != nil {
				t.Fatal(err)
			}
			if data.Outcome != tt.outcome || data.PeerUID != "peer1" || data.Reason != "resource_deleted" {
				t.Fatalf("unexpected data: %+v", data)
			}
			if strings.Contains(string(event.Data()), "private diagnostic") {
				t.Fatal("raw device errors leaked into event")
			}
		})
	}
}

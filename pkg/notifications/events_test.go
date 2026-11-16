// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/openchami/fabrica/pkg/events"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type captureBus struct {
	events []events.Event
	err    error
}

func (*captureBus) Start()       {}
func (*captureBus) Close() error { return nil }
func (*captureBus) Subscribe(string, events.EventHandler) (events.SubscriptionID, error) {
	return "", nil
}
func (*captureBus) Unsubscribe(events.SubscriptionID) error { return nil }
func (b *captureBus) Publish(_ context.Context, event events.Event) error {
	b.events = append(b.events, event)
	return b.err
}

func TestEmitProducesStandaloneCloudEventEvenWhenBusFails(t *testing.T) {
	previousBus, previousOutput := events.GetGlobalEventBus(), output.Writer()
	t.Cleanup(func() { events.SetGlobalEventBus(previousBus); output.SetOutput(previousOutput) })
	var buffer bytes.Buffer
	output.SetOutput(&buffer)
	bus := &captureBus{err: errors.New("queue full")}
	events.SetGlobalEventBus(bus)
	id, err := Emit(context.Background(), PhoneHomeReceived, "nodes/node1", PhoneHome{NodeID: "node1", IdentityBasis: "reported"})
	if err == nil || id == "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	line := strings.TrimSpace(buffer.String())
	if strings.Contains(line, "\n") || !json.Valid([]byte(line)) {
		t.Fatalf("not one JSON event: %q", line)
	}
	var event events.Event
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatal(err)
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if event.ID() != id || event.Type() != PhoneHomeReceived || event.Source() != "/metadata-service" || event.Subject() != "nodes/node1" || event.Time().IsZero() {
		t.Fatalf("unexpected envelope: %s", event.String())
	}
	if len(bus.events) != 1 || bus.events[0].ID() != id {
		t.Fatal("stdout and bus records must use the same event ID")
	}
}

func TestEmitReportsLogWriteFailureAndStillOffersToBus(t *testing.T) {
	previousBus, previousOutput := events.GetGlobalEventBus(), output.Writer()
	t.Cleanup(func() { events.SetGlobalEventBus(previousBus); output.SetOutput(previousOutput) })
	output.SetOutput(failingWriter{})
	bus := &captureBus{}
	events.SetGlobalEventBus(bus)
	id, err := Emit(context.Background(), WireGuardShutdown, "wireguardpeers/peer1", Shutdown{PeerUID: "peer1", Reason: "resource_deleted", Outcome: "removed"})
	if !errors.Is(err, io.ErrClosedPipe) || id == "" || len(bus.events) != 1 {
		t.Fatalf("id=%q err=%v, events=%d", id, err, len(bus.events))
	}
}

// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

// Package notifications emits service notifications without storing event history.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"

	"github.com/openchami/fabrica/pkg/events"
)

const (
	// PhoneHomeReceived records a report, not proof of successful provisioning.
	PhoneHomeReceived = "org.openchami.metadata.phonehome.received.v1"
	// WireGuardShutdown records the outcome of a controller removal attempt.
	WireGuardShutdown = "org.openchami.metadata.wireguard.shutdown.v1"
)

// PhoneHome describes a bounded, explicitly attributed callback receipt.
type PhoneHome struct {
	NodeID        string `json:"node_id"`
	InstanceID    string `json:"instance_id,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	FQDN          string `json:"fqdn,omitempty"`
	TransportIP   string `json:"transport_ip,omitempty"`
	IdentityBasis string `json:"identity_basis"`
	RequestID     string `json:"request_id,omitempty"`
	PeerUID       string `json:"peer_uid,omitempty"`
}

// Shutdown carries correlation from the removal request to the actual result.
type Shutdown struct {
	PeerUID          string `json:"peer_uid,omitempty"`
	Reason           string `json:"reason"`
	Outcome          string `json:"outcome,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	PhoneHomeEventID string `json:"phone_home_event_id,omitempty"`
}

type shutdownKey struct{}

// WithShutdown attaches trusted service-generated request correlation.
func WithShutdown(ctx context.Context, details Shutdown) context.Context {
	return context.WithValue(ctx, shutdownKey{}, details)
}

// ShutdownFromContext supplies defaults for removals outside the callback path.
func ShutdownFromContext(ctx context.Context, uid string) Shutdown {
	details, ok := ctx.Value(shutdownKey{}).(Shutdown)
	if !ok {
		details.Reason = "resource_deleted"
	}
	details.PeerUID = uid
	return details
}

// A separate, unprefixed stream keeps each notification a complete JSON CloudEvent.
// Host logging/collectors own retention, rotation, and forwarding.
var output = log.New(os.Stdout, "", 0)

// Emit logs a CloudEvent and offers it to the existing in-process event bus.
// Neither the log write nor an asynchronous enqueue promises durable delivery.
func Emit(ctx context.Context, eventType, subject string, data any) (string, error) {
	event, err := events.NewEvent(eventType, "/metadata-service", data)
	if err != nil {
		return "", err
	}
	event.SetSubject(subject)
	encoded, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	writeErr := output.Output(2, string(encoded))
	var publishErr error
	if bus := events.GetGlobalEventBus(); bus != nil {
		publishErr = bus.Publish(ctx, *event)
	}
	return event.ID(), errors.Join(writeErr, publishErr)
}

// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openchami/fabrica/pkg/events"
	"github.com/openchami/metadata-service/internal/storage"
	"github.com/openchami/metadata-service/pkg/notifications"
	"github.com/openchami/metadata-service/pkg/reconcilers"
	"github.com/spf13/viper"
)

type notificationCapture struct {
	mu            sync.Mutex
	events        []events.Event
	failLifecycle bool
}

func (b *notificationCapture) Start()       {}
func (b *notificationCapture) Close() error { return nil }
func (b *notificationCapture) Subscribe(string, events.EventHandler) (events.SubscriptionID, error) {
	return "", nil
}
func (b *notificationCapture) Unsubscribe(events.SubscriptionID) error { return nil }
func (b *notificationCapture) Publish(_ context.Context, event events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failLifecycle && event.ResourceKind() != "" {
		return errors.New("queue full")
	}
	b.events = append(b.events, event)
	return nil
}
func (b *notificationCapture) snapshot() []events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]events.Event{}, b.events...)
}
func newNotificationCapture(t *testing.T) *notificationCapture {
	t.Helper()
	previousBus, previousConfig := events.GetGlobalEventBus(), events.GetEventConfig()
	bus := &notificationCapture{}
	cfg := events.DefaultEventConfig()
	cfg.Enabled, cfg.LifecycleEventsEnabled = true, true
	events.SetEventConfig(cfg)
	events.SetGlobalEventBus(bus)
	t.Cleanup(func() {
		events.SetGlobalEventBus(previousBus)
		events.SetEventConfig(previousConfig)
	})
	return bus
}
func capturedEvent(t *testing.T, bus *notificationCapture, eventType string) events.Event {
	t.Helper()
	matches := []events.Event{}
	for _, event := range bus.snapshot() {
		if event.Type() == eventType {
			matches = append(matches, event)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("got %d events of type %s, want 1", len(matches), eventType)
	}
	if err := matches[0].Validate(); err != nil {
		t.Fatalf("invalid CloudEvent: %v", err)
	}
	return matches[0]
}

func TestPhoneHomeFormProducesSanitizedReceipt(t *testing.T) {
	bus := newNotificationCapture(t)
	router := chi.NewRouter()
	router.Use(middleware.RequestID, captureTransportPeer, middleware.RealIP)
	registerPhoneHomeRoute(router, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/phone-home/node-1", strings.NewReader(
		"instance_id=instance-1&hostname=node1&fqdn=node1.example&pub_key_ed25519=excluded-key&token=excluded-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer excluded-credential")
	req.Header.Set("X-Forwarded-For", "100.97.0.8")
	req.RemoteAddr = "192.0.2.9:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	event := capturedEvent(t, bus, notifications.PhoneHomeReceived)
	var receipt notifications.PhoneHome
	if err := event.DataAs(&receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.NodeID != "node-1" || receipt.InstanceID != "instance-1" || receipt.Hostname != "node1" || receipt.FQDN != "node1.example" {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if receipt.TransportIP != "192.0.2.9" || receipt.IdentityBasis != "reported" || receipt.RequestID == "" {
		t.Fatalf("incorrect attribution: %+v", receipt)
	}
	if event.Time().IsZero() || event.ID() == "" || event.Subject() != "nodes/node-1" {
		t.Fatalf("incomplete envelope: %s", event.String())
	}
	if strings.Contains(string(event.Data()), "excluded-") {
		t.Fatalf("unselected/secret data in event: %s", event.Data())
	}
	if len(bus.snapshot()) != 1 {
		t.Fatal("report-only callback must not publish cleanup events")
	}
}

func TestPhoneHomeRejectsInvalidPayload(t *testing.T) {
	tests := []struct {
		name, body, contentType, path string
		status                        int
	}{
		{"oversized", "hostname=" + strings.Repeat("x", maxPhoneHomeBody), "application/x-www-form-urlencoded", "/phone-home/node1", http.StatusRequestEntityTooLarge},
		{"duplicate", "hostname=a&hostname=b", "application/x-www-form-urlencoded", "/phone-home/node1", http.StatusBadRequest},
		{"invalid escape", "hostname=%xx", "application/x-www-form-urlencoded", "/phone-home/node1", http.StatusBadRequest},
		{"newline", "hostname=a%0Ab", "application/x-www-form-urlencoded", "/phone-home/node1", http.StatusBadRequest},
		{"json", "{}", "application/json", "/phone-home/node1", http.StatusUnsupportedMediaType},
		{"long identifier", "", "", "/phone-home/" + strings.Repeat("a", 257), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := newNotificationCapture(t)
			router := chi.NewRouter()
			registerPhoneHomeRoute(router, nil, nil)
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.status {
				t.Fatalf("got %d, want %d: %s", response.Code, tt.status, response.Body.String())
			}
			if len(bus.snapshot()) != 0 {
				t.Fatal("invalid callback emitted an event")
			}
		})
	}
}

func TestPhoneHomeShutdownCorrelatesWithControllerResult(t *testing.T) {
	initTestStorageBackend(t)
	bus := newNotificationCapture(t)
	controller := newTestWireGuardController(t)
	const uid = "wireguardpeer-owned"
	saveWireGuardPeerFixture(t, uid, "node-owned", testPublicKeyA, "100.97.0.8/32")
	if err := controller.UpsertPeer(uid, testPublicKeyA, "100.97.0.8/32"); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID, captureTransportPeer, middleware.RealIP)
	registerPhoneHomeRoute(router, controller, nil)
	req := httptest.NewRequest(http.MethodPost, "/phone-home/"+uid, nil)
	req.RemoteAddr = "100.97.0.8:1234"
	req.Header.Set("X-Forwarded-For", "192.0.2.99")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	receipt := capturedEvent(t, bus, notifications.PhoneHomeReceived)
	var receiptData notifications.PhoneHome
	if err := receipt.DataAs(&receiptData); err != nil {
		t.Fatal(err)
	}
	if receiptData.IdentityBasis != "wireguard_address" || receiptData.PeerUID != uid {
		t.Fatalf("unexpected receipt: %+v", receiptData)
	}
	if _, found := controller.GetPeerVPNIP(uid); !found {
		t.Fatal("receipt must not claim synchronous tunnel removal")
	}
	for _, e := range bus.snapshot() {
		if e.Type() == notifications.WireGuardShutdown {
			t.Fatal("shutdown event published before removal")
		}
	}
	runtime := &reconciliationRuntime{
		wireGuardController:     controller,
		wireGuardPeerReconciler: reconcilers.NewWireGuardPeerReconciler(storage.NewStorageClient(), controller),
	}
	deletion := capturedEvent(t, bus, "io.fabrica.wireguardpeer.deleted")
	var legacy events.ResourceChangeData
	if err := deletion.DataAs(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Metadata["phoneHomeID"] != uid {
		t.Fatal("legacy phoneHomeID metadata lost")
	}
	if err := runtime.handleResourceEvent(context.Background(), deletion); err != nil {
		t.Fatal(err)
	}
	if _, found := controller.GetPeerVPNIP(uid); found {
		t.Fatal("controller peer still present")
	}
	shutdown := capturedEvent(t, bus, notifications.WireGuardShutdown)
	var details notifications.Shutdown
	if err := shutdown.DataAs(&details); err != nil {
		t.Fatal(err)
	}
	if details.Outcome != "removed" || details.PeerUID != uid || details.Reason != "phone_home" || details.PhoneHomeEventID != receipt.ID() || details.RequestID != receiptData.RequestID {
		t.Fatalf("incorrect shutdown correlation: %+v", details)
	}
}

func TestPhoneHomeQueueFailureRestoresIntent(t *testing.T) {
	initTestStorageBackend(t)
	bus := newNotificationCapture(t)
	bus.failLifecycle = true
	controller := newTestWireGuardController(t)
	const uid = "wireguardpeer-owned"
	saveWireGuardPeerFixture(t, uid, "node-owned", testPublicKeyA, "100.97.0.8/32")
	if err := controller.UpsertPeer(uid, testPublicKeyA, "100.97.0.8/32"); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	registerPhoneHomeRoute(router, controller, nil)
	req := httptest.NewRequest(http.MethodPost, "/phone-home/"+uid, nil)
	req.RemoteAddr = "100.97.0.8:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", response.Code)
	}
	if _, err := storage.LoadWireGuardPeer(context.Background(), uid); err != nil {
		t.Fatalf("intent lost after failed enqueue: %v", err)
	}
	if len(bus.snapshot()) != 1 {
		t.Fatal("only the receipt should have been emitted")
	}
}

func TestPhoneHomeRegisteredWithoutWireGuard(t *testing.T) {
	initTestStorageBackend(t)
	newNotificationCapture(t)
	viper.Reset()
	t.Cleanup(viper.Reset)
	previousMock, previousHealth, previousWG := mockSMD, currentSMDHealth, currentWGController
	mockSMD = true
	t.Cleanup(func() { mockSMD, currentSMDHealth, currentWGController = previousMock, previousHealth, previousWG })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	router := chi.NewRouter()
	if err := registerCustomServerIntegrations(ctx, router); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/phone-home/node-1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("got %d, want phone-home without WG", response.Code)
	}
	wgResponse := httptest.NewRecorder()
	router.ServeHTTP(wgResponse, httptest.NewRequest(http.MethodPost, "/wg-init", nil))
	if wgResponse.Code != http.StatusNotFound {
		t.Fatalf("WG allocation exposed while disabled: %d", wgResponse.Code)
	}
}

func TestWireGuardOnlyRequiresController(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	previous := currentWGController
	t.Cleanup(func() { currentWGController = previous })
	viper.Set("wireguard_only", true)
	if err := registerCustomServerIntegrations(context.Background(), chi.NewRouter()); err == nil {
		t.Fatal("wireguard_only silently disabled without a controller")
	}
}

func TestWireGuardOnlyUsesTransportBeforeRealIP(t *testing.T) {
	tests := []struct {
		name, peer, forwarded string
		status                int
	}{
		{"spoofed tunnel source", "192.0.2.9:1234", "100.97.0.8", http.StatusForbidden},
		{"tunnel source with outside header", "100.97.0.8:1234", "192.0.2.9", http.StatusOK},
		{"proxy is not a tunnel peer", "127.0.0.1:1234", "100.97.0.8", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := chi.NewRouter()
			router.Use(captureTransportPeer, middleware.RealIP, wireGuardOnlyMiddleware(newTestWireGuardController(t)))
			registerPhoneHomeRoute(router, nil, nil)
			req := httptest.NewRequest(http.MethodPost, "/phone-home/node-1", nil)
			req.RemoteAddr = tt.peer
			req.Header.Set("X-Forwarded-For", tt.forwarded)
			req.Header.Set("X-Real-IP", tt.forwarded)
			req.Header.Set("Forwarded", "for="+tt.forwarded)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.status {
				t.Fatalf("got %d, want %d", response.Code, tt.status)
			}
		})
	}
}

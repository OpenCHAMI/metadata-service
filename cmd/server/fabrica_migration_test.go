// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openchami/fabrica/pkg/events"
	"github.com/openchami/fabrica/pkg/fabrica"
	"github.com/spf13/viper"

	v1 "github.com/openchami/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/openchami/metadata-service/internal/storage"
	"github.com/openchami/metadata-service/pkg/wireguard"
)

// newFeatureRouterForTest builds the real router with generated features but
// stubbed custom integrations, so only generated routes are protected.
// protectedMiddleware is applied to the protected group, as the WireGuard
// middleware is by registerCustomServerIntegrations.
func newFeatureRouterForTest(t *testing.T, protectedMiddleware ...func(http.Handler) http.Handler) chi.Router {
	t.Helper()

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	previousEvents := events.GetEventConfig()
	features, err := loadGeneratedFeatures(context.Background(), cfg)
	if err != nil {
		t.Fatalf("loadGeneratedFeatures() failed: %v", err)
	}
	t.Cleanup(func() {
		features.Close()
		events.SetEventConfig(previousEvents)
	})

	origRegister := registerServerIntegrations
	registerServerIntegrations = func(_ context.Context, r chi.Router) error {
		if err := registerResourcePrefixesSafely(); err != nil {
			return err
		}
		r.Use(protectedMiddleware...)
		RegisterGeneratedRoutes(r)
		return nil
	}
	t.Cleanup(func() { registerServerIntegrations = origRegister })

	r, err := newRouter(context.Background(), cfg, features)
	if err != nil {
		t.Fatalf("newRouter() failed: %v", err)
	}
	return r
}

func serveForTest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGeneratedFeaturesServeMetricsOnMainRouter(t *testing.T) {
	r := newFeatureRouterForTest(t)

	if w := serveForTest(r, http.MethodGet, "/health", ""); w.Code != http.StatusOK {
		t.Fatalf("GET /health returned HTTP %d", w.Code)
	}
	w := serveForTest(r, http.MethodGet, "/metrics", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics returned HTTP %d", w.Code)
	}
	// The generated namespace replaces the legacy scaffold's "main".
	if !strings.Contains(w.Body.String(), "metadata_service_http_request_total") {
		t.Fatalf("expected metadata_service_* HTTP metrics after a request, got:\n%s", w.Body.String())
	}
}

func TestDedicatedMetricsListenerServesGeneratedMetrics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	features, err := loadGeneratedFeatures(context.Background(), cfg)
	if err != nil {
		t.Fatalf("loadGeneratedFeatures() failed: %v", err)
	}
	defer features.Close()

	handler := newMetricsHandler(features)
	if w := serveForTest(handler, http.MethodGet, "/metrics", ""); w.Code != http.StatusOK {
		t.Fatalf("GET /metrics on dedicated listener returned HTTP %d", w.Code)
	}
	if w := serveForTest(handler, http.MethodGet, "/groups", ""); w.Code != http.StatusNotFound {
		t.Fatalf("dedicated metrics listener must not serve API routes, got HTTP %d", w.Code)
	}
}

func TestGeneratedFeaturesStartEventBusWithLegacyIdentity(t *testing.T) {
	_ = newFeatureRouterForTest(t)
	applyEventIdentityDefaults()

	if events.GetGlobalEventBus() == nil {
		t.Fatal("expected generated features to start the global event bus")
	}
	cfg := events.GetEventConfig()
	if !cfg.Enabled || !cfg.LifecycleEventsEnabled {
		t.Fatalf("expected lifecycle events enabled, got %+v", cfg)
	}
	if cfg.EventTypePrefix != legacyEventTypePrefix || cfg.ConditionEventPrefix != legacyEventTypePrefix+".condition" || cfg.Source != eventSource {
		t.Fatalf("unexpected event identity %+v", cfg)
	}
}

func TestEventIdentityHonorsExplicitTypePrefix(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("event_type_prefix", "org.example")

	_ = newFeatureRouterForTest(t)
	applyEventIdentityDefaults()

	cfg := events.GetEventConfig()
	if cfg.EventTypePrefix != "org.example" || cfg.ConditionEventPrefix != "org.example.condition" {
		t.Fatalf("expected explicit event_type_prefix to be honored, got %+v", cfg)
	}
}

func TestDataDirFlagKeepsAbsoluteDefault(t *testing.T) {
	flag := serveCmd.Flags().Lookup("data-dir")
	if flag == nil {
		t.Fatal("expected generated --data-dir flag")
	}
	if flag.DefValue != defaultDataDir || flag.Value.String() != defaultDataDir {
		t.Fatalf("expected --data-dir default %q, got DefValue %q value %q", defaultDataDir, flag.DefValue, flag.Value.String())
	}
	if flag.Changed {
		t.Fatal("overriding the default must not mark --data-dir as changed")
	}
}

func TestCreateRequiresSpecEnvelope(t *testing.T) {
	r := newFeatureRouterForTest(t)

	for _, path := range []string{"/clusterdefaultss", "/groups", "/instanceinfos", "/wireguardpeers"} {
		for _, body := range []string{`{"metadata":{"name":"x"}}`, `{"metadata":{"name":"x"},"spec":null}`} {
			if w := serveForTest(r, http.MethodPost, path, body); w.Code != http.StatusBadRequest {
				t.Errorf("POST %s %s returned HTTP %d, want 400: %s", path, body, w.Code, w.Body)
			}
		}
	}
}

// Stores written by earlier releases hold prefix-hex UIDs, which are never
// rewritten and must stay fully usable.
func TestLegacyUIDsRemainUsable(t *testing.T) {
	r := newFeatureRouterForTest(t)

	const legacyUID = "group-1a2b3c4d"
	legacy := &v1.Group{
		APIVersion: "v1",
		Kind:       "Group",
		Metadata:   fabrica.Metadata{UID: legacyUID, Name: "legacy"},
		Spec:       v1.GroupSpec{Template: "#cloud-config\n"},
	}
	if err := storage.SaveGroup(context.Background(), legacy); err != nil {
		t.Fatalf("SaveGroup() failed: %v", err)
	}

	path := "/groups/" + legacyUID
	if w := serveForTest(r, http.MethodGet, path, ""); w.Code != http.StatusOK {
		t.Fatalf("GET returned HTTP %d: %s", w.Code, w.Body)
	}
	if w := serveForTest(r, http.MethodDelete, path, ""); w.Code != http.StatusOK {
		t.Fatalf("DELETE returned HTTP %d: %s", w.Code, w.Body)
	}
	if w := serveForTest(r, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE returned HTTP %d, want 404", w.Code)
	}
}

// WireGuard-only mode applies only to the protected group. The public routes,
// including the generated /metrics, stay reachable from any address; sites that
// need them private must block them at the network or proxy. This test pins the
// list so moving a route in or out of the public group is a deliberate change.
func TestWireGuardOnlyModeDoesNotCoverPublicRoutes(t *testing.T) {
	_, network, err := net.ParseCIDR("100.97.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	r := newFeatureRouterForTest(t, wireGuardOnlyMiddleware(&wireguard.Controller{Network: *network}))

	request := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.10:4321" // outside the WireGuard network
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	for _, path := range []string{"/health", "/service/status", "/.well-known/api-catalog", "/openapi.json", "/docs", "/metrics"} {
		if code := request(path); code == http.StatusForbidden {
			t.Errorf("GET %s from outside WireGuard returned 403; public routes are documented as bypassing wireguard_only", path)
		}
	}
	if code := request("/groups"); code != http.StatusForbidden {
		t.Errorf("GET /groups from outside WireGuard returned HTTP %d, want 403", code)
	}
}

// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openchami/metadata-service/pkg/smdclient"
)

func TestServiceStatusReflectsSMDHealth(t *testing.T) {
	tests := []struct {
		name       string
		health     smdHealthReporter
		wantCode   int
		wantStatus ServiceStatus
	}{
		{name: "no SMD health reporter", health: nil, wantCode: http.StatusOK, wantStatus: ServiceStatusOK},
		{name: "healthy", health: stubSMDHealthReporter{healthy: true}, wantCode: http.StatusOK, wantStatus: ServiceStatusOK},
		{
			name:       "initial refresh pending",
			health:     stubSMDHealthReporter{reason: smdclient.InitialRefreshPendingReason},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: ServiceStatusInitializing,
		},
		{
			name:       "auth failure",
			health:     stubSMDHealthReporter{reason: "token refresh failed"},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: ServiceStatusDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := currentSMDHealth
			currentSMDHealth = tt.health
			t.Cleanup(func() { currentSMDHealth = previous })

			r := chi.NewRouter()
			RegisterGeneratedPublicRoutes(r)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/service/status", nil))

			if w.Code != tt.wantCode {
				t.Fatalf("expected HTTP %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}
			var payload ServiceStatusResponse
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload.Status != tt.wantStatus {
				t.Fatalf("expected status %q, got %q", tt.wantStatus, payload.Status)
			}
			if payload.Service != "metadata-service" {
				t.Fatalf("expected service metadata-service, got %q", payload.Service)
			}
		})
	}
}

func TestAPICatalogIsPublic(t *testing.T) {
	r := chi.NewRouter()
	RegisterGeneratedPublicRoutes(r)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, "/.well-known/api-catalog", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", method, w.Code)
		}
		if got := w.Header().Get("Link"); got == "" {
			t.Fatalf("%s: expected api-catalog Link header", method)
		}
	}
}

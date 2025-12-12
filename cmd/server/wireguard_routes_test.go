// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Only test nil-controller and input validation paths to avoid real device setup.

func TestWGInitNoController(t *testing.T) {
	router := chi.NewRouter()
	registerWireGuardRoutes(router, nil, nil)

	body := bytes.NewBufferString(`{"public_key":"test"}`)
	req := httptest.NewRequest("POST", "/wg-init", body)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestPhoneHomeNoController(t *testing.T) {
	router := chi.NewRouter()
	registerWireGuardRoutes(router, nil, nil)

	req := httptest.NewRequest("POST", "/phone-home/node1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
)

// TestWireGuardOnlyMiddlewareWithValidIP tests in-network access passes through.
func TestWireGuardOnlyMiddlewareWithValidIP(t *testing.T) {
	_, network, _ := net.ParseCIDR("100.97.0.0/16")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("allowed"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "100.97.1.5:1234"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for in-network IP, got %d", w.Code)
	}
	if body := w.Body.String(); body != "allowed" {
		t.Fatalf("expected 'allowed', got %s", body)
	}
}

// TestWireGuardOnlyMiddlewareWithInvalidIP tests out-of-network access denied.
func TestWireGuardOnlyMiddlewareWithInvalidIP(t *testing.T) {
	_, network, _ := net.ParseCIDR("100.97.0.0/16")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for out-of-network IP, got %d", w.Code)
	}
}

// TestWireGuardOnlyMiddlewareWithNilController allows all when controller is nil.
func TestWireGuardOnlyMiddlewareWithNilController(t *testing.T) {
	handler := wireGuardOnlyMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when controller is nil, got %d", w.Code)
	}
}

// TestWireGuardOnlyMiddlewareEdgeCaseLocalhost tests localhost access.
func TestWireGuardOnlyMiddlewareEdgeCaseLocalhost(t *testing.T) {
	_, network, _ := net.ParseCIDR("127.0.0.0/8")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for localhost in allowed network, got %d", w.Code)
	}
}

// TestWireGuardOnlyMiddlewareIPv6 tests IPv6 address handling.
func TestWireGuardOnlyMiddlewareIPv6(t *testing.T) {
	_, network, _ := net.ParseCIDR("2001:db8::/32")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[2001:db8::1]:8080"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for IPv6 in allowed network, got %d", w.Code)
	}
}

// TestWireGuardOnlyMiddlewareRejectsIPv6OutOfNetwork tests IPv6 rejection.
func TestWireGuardOnlyMiddlewareRejectsIPv6OutOfNetwork(t *testing.T) {
	_, network, _ := net.ParseCIDR("2001:db8::/32")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[2001:db9::1]:8080"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for IPv6 out-of-network, got %d", w.Code)
	}
}

// TestWireGuardOnlyMiddlewareInvalidRemoteAddr tests handling of malformed RemoteAddr.
func TestWireGuardOnlyMiddlewareInvalidRemoteAddr(t *testing.T) {
	_, network, _ := net.ParseCIDR("100.97.0.0/16")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "not-an-ip"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid IP, got %d", w.Code)
	}
}

// TestWireGuardOnlyMiddlewareNetworkBoundary tests exact network boundaries.
func TestWireGuardOnlyMiddlewareNetworkBoundary(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.0.0/24")
	ctrl := &wireguard.Controller{Network: *network}

	handler := wireGuardOnlyMiddleware(ctrl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test network address (allowed)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.0.0:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for network address, got %d", w.Code)
	}

	// Test broadcast address (allowed)
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.0.255:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for broadcast address, got %d", w.Code)
	}

	// Test just outside network (denied)
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.0:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for IP outside network, got %d", w.Code)
	}
}

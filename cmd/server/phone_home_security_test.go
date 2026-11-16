// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openchami/metadata-service/internal/storage"
)

func TestPhoneHomeOutsideTunnelCannotDeletePeer(t *testing.T) {
	initTestStorageBackend(t)
	controller := newTestWireGuardController(t)
	const uid = "wireguardpeer-victim"
	saveWireGuardPeerFixture(t, uid, "node-victim", testPublicKeyA, "100.97.0.8/32")
	router := chi.NewRouter()
	registerWireGuardRoutes(router, controller, nil)
	registerPhoneHomeRoute(router, controller, nil)
	request := httptest.NewRequest(http.MethodPost, "/phone-home/"+uid, nil)
	request.RemoteAddr = "192.0.2.5:1234"
	request.Header.Set("X-Forwarded-For", "100.97.0.8")
	request.Header.Set("Forwarded", "for=100.97.0.8")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected report-only 200, got %d: %s", response.Code, response.Body.String())
	}
	if _, err := storage.LoadWireGuardPeer(context.Background(), uid); err != nil {
		t.Fatalf("outside caller removed peer: %v", err)
	}
}

func TestPhoneHomeCannotDeleteAnotherTunnelPeer(t *testing.T) {
	initTestStorageBackend(t)
	controller := newTestWireGuardController(t)
	const uid = "wireguardpeer-victim"
	saveWireGuardPeerFixture(t, uid, "node-victim", testPublicKeyA, "100.97.0.8/32")
	router := chi.NewRouter()
	registerWireGuardRoutes(router, controller, nil)
	registerPhoneHomeRoute(router, controller, nil)
	request := httptest.NewRequest(http.MethodPost, "/phone-home/"+uid, nil)
	request.RemoteAddr = "100.97.0.9:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected ownership rejection, got %d: %s", response.Code, response.Body.String())
	}
	if _, err := storage.LoadWireGuardPeer(context.Background(), uid); err != nil {
		t.Fatalf("another peer was removed: %v", err)
	}
}

func TestPhoneHomeLoopbackProxyCannotRemovePeer(t *testing.T) {
	initTestStorageBackend(t)
	bus := newNotificationCapture(t)
	controller := newTestWireGuardController(t)
	const uid = "wireguardpeer-victim"
	saveWireGuardPeerFixture(t, uid, "node-victim", testPublicKeyA, "100.97.0.8/32")
	router := chi.NewRouter()
	router.Use(captureTransportPeer, middleware.RealIP)
	registerPhoneHomeRoute(router, controller, nil)
	backend := httptest.NewServer(router)
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	for _, address := range []string{backend.URL, proxy.URL} {
		req, err := http.NewRequest(http.MethodPost, address+"/phone-home/"+uid, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-For", "100.97.0.8")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("body read=%v close=%v", readErr, closeErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("got %d", response.StatusCode)
		}
	}
	if _, err := storage.LoadWireGuardPeer(context.Background(), uid); err != nil {
		t.Fatalf("peer removed by forwarded address: %v", err)
	}
	if len(bus.snapshot()) != 2 {
		t.Fatalf("expected two report events, got %d", len(bus.snapshot()))
	}
}

func TestPhoneHomeInactivePeerCannotRequestShutdown(t *testing.T) {
	initTestStorageBackend(t)
	controller := newTestWireGuardController(t)
	const uid = "wireguardpeer-inactive"
	saveWireGuardPeerFixture(t, uid, "node-inactive", testPublicKeyA, "100.97.0.8/32")
	router := chi.NewRouter()
	registerPhoneHomeRoute(router, controller, nil)
	request := httptest.NewRequest(http.MethodPost, "/phone-home/"+uid, nil)
	request.RemoteAddr = "100.97.0.8:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("inactive peer authorized cleanup: %d", response.Code)
	}
	if _, err := storage.LoadWireGuardPeer(context.Background(), uid); err != nil {
		t.Fatal(err)
	}
}

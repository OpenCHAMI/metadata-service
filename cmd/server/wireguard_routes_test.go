// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenCHAMI/cloud-init/internal/storage"
	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
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

type mockWireGuardDevice struct {
	pubKey string
	peers  map[string]string
}

func (m *mockWireGuardDevice) SetPrivateKey(_ string) error { return nil }
func (m *mockWireGuardDevice) AddPeer(publicKey, allowedIP string) error {
	if m.peers == nil {
		m.peers = make(map[string]string)
	}
	m.peers[publicKey] = allowedIP
	return nil
}
func (m *mockWireGuardDevice) RemovePeer(publicKey string) error {
	delete(m.peers, publicKey)
	return nil
}
func (m *mockWireGuardDevice) Close() error                     { return nil }
func (m *mockWireGuardDevice) PublicKeyValue() string           { return m.pubKey }
func (m *mockWireGuardDevice) SetPublicKeyValue(pub string)     { m.pubKey = pub }
func (m *mockWireGuardDevice) ListenPortValue() int             { return 51820 }
func (m *mockWireGuardDevice) PrivateKeyValue() (string, error) { return "", nil }

func TestWGInitThenPhoneHomeRemovesPeer(t *testing.T) {
	t.Helper()

	if err := storage.InitFileBackend(t.TempDir()); err != nil {
		t.Fatalf("init storage backend: %v", err)
	}

	_, network, err := net.ParseCIDR("10.252.0.0/24")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	allocator, err := wireguard.NewIPAllocator(network.String())
	if err != nil {
		t.Fatalf("new allocator: %v", err)
	}
	serverIP := net.ParseIP("10.252.0.254")
	if err := allocator.Reserve(net.IPAddr{IP: serverIP}); err != nil {
		t.Fatalf("reserve server ip: %v", err)
	}

	dev := &mockWireGuardDevice{pubKey: "server-pub", peers: make(map[string]string)}
	controller := &wireguard.Controller{
		Device:      dev,
		Network:     *network,
		ServerIP:    serverIP,
		Peers:       make(map[string]wireguard.PeerState),
		IPAllocator: allocator,
	}

	router := chi.NewRouter()
	registerWireGuardRoutes(router, controller, nil)

	const clientIP = "10.0.0.100"
	const publicKey = "client-public-key"
	peerUID := peerUIDForKey(publicKey)

	wgInitReq := bytes.NewBufferString(`{"public_key":"` + publicKey + `"}`)
	req := httptest.NewRequest("POST", "/wg-init", wgInitReq)
	req.Header.Set("X-Forwarded-For", clientIP)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 from /wg-init, got %d body=%s", w.Code, w.Body.String())
	}

	var initResp wgInitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("unmarshal /wg-init response: %v", err)
	}
	if initResp.ClientVPNIP == "" {
		t.Fatalf("expected non-empty client VPN IP")
	}

	if _, ok := controller.GetPeerVPNIP(peerUID); !ok {
		t.Fatalf("expected peer %s to exist in controller after /wg-init", peerUID)
	}

	phoneReq := httptest.NewRequest("POST", "/phone-home/not-a-uid", nil)
	phoneReq.Header.Set("X-Forwarded-For", clientIP)
	phoneResp := httptest.NewRecorder()
	router.ServeHTTP(phoneResp, phoneReq)
	if phoneResp.Code != http.StatusOK {
		t.Fatalf("expected 200 from /phone-home, got %d", phoneResp.Code)
	}

	if _, ok := controller.GetPeerVPNIP(peerUID); ok {
		t.Fatalf("expected peer %s to be removed after /phone-home", peerUID)
	}
}

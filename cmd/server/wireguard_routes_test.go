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

	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/OpenCHAMI/metadata-service/pkg/wireguard"
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

type fakeWireGuardDevice struct {
	publicKey   string
	listenPort  int
	addedPeers  map[string]string
	removedKeys []string
}

func (d *fakeWireGuardDevice) SetPrivateKey(string) error       { return nil }
func (d *fakeWireGuardDevice) Close() error                     { return nil }
func (d *fakeWireGuardDevice) PublicKeyValue() string           { return d.publicKey }
func (d *fakeWireGuardDevice) SetPublicKeyValue(pub string)     { d.publicKey = pub }
func (d *fakeWireGuardDevice) ListenPortValue() int             { return d.listenPort }
func (d *fakeWireGuardDevice) PrivateKeyValue() (string, error) { return "", nil }
func (d *fakeWireGuardDevice) AddPeer(pub, allowed string) error {
	d.addedPeers[pub] = allowed
	return nil
}
func (d *fakeWireGuardDevice) RemovePeer(publicKey string) error {
	d.removedKeys = append(d.removedKeys, publicKey)
	return nil
}

func newTestController(t *testing.T) *wireguard.Controller {
	t.Helper()
	_, network, err := net.ParseCIDR("100.97.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDR failed: %v", err)
	}
	allocator, err := wireguard.NewIPAllocator(network.String())
	if err != nil {
		t.Fatalf("NewIPAllocator failed: %v", err)
	}
	serverIP := net.ParseIP("100.97.0.1")
	if err := allocator.Reserve(net.IPAddr{IP: serverIP}); err != nil {
		t.Fatalf("reserve server IP failed: %v", err)
	}
	device := &fakeWireGuardDevice{
		publicKey:  "server-public-key",
		listenPort: 51820,
		addedPeers: make(map[string]string),
	}
	return &wireguard.Controller{
		Device:      device,
		Network:     *network,
		ServerIP:    serverIP,
		Peers:       make(map[string]wireguard.PeerState),
		IPAllocator: allocator,
	}
}

func TestWireGuardRoutesHappyPathWithSMDIntegration(t *testing.T) {
	controller := newTestController(t)
	mockSMD := smdclient.NewMockSMDClient()
	mockSMD.AddComponent(&smdclient.Component{
		ID:  "x1000c0s0b0n0",
		IP:  "10.0.0.7",
		NID: 1000,
	})

	router := chi.NewRouter()
	registerWireGuardRoutes(router, controller, mockSMD)

	req := httptest.NewRequest(http.MethodPost, "/wg-init", bytes.NewBufferString(`{"public_key":"client-pub-key"}`))
	req.Header.Set("X-Forwarded-For", "10.0.0.7")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp wgInitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode wg-init response: %v", err)
	}
	if resp.ClientVPNIP == "" {
		t.Fatal("expected allocated VPN IP in response")
	}
	if resp.ServerPubKey != "server-public-key" {
		t.Fatalf("expected server public key in response, got %q", resp.ServerPubKey)
	}

	storedWGIP, err := mockSMD.WGIPfromID("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("expected AddWGIP side effect, got error: %v", err)
	}
	if storedWGIP != resp.ClientVPNIP {
		t.Fatalf("expected WG IP %q in SMD store, got %q", resp.ClientVPNIP, storedWGIP)
	}

	device := controller.Device.(*fakeWireGuardDevice)
	if got := device.addedPeers["client-pub-key"]; got != resp.ClientVPNIP+"/32" {
		t.Fatalf("expected AddPeer allowed IP %q, got %q", resp.ClientVPNIP+"/32", got)
	}

	phoneHome := httptest.NewRequest(http.MethodPost, "/phone-home/x1000c0s0b0n0", nil)
	phoneHome.Header.Set("X-Forwarded-For", "10.0.0.7")
	phoneHomeResp := httptest.NewRecorder()
	router.ServeHTTP(phoneHomeResp, phoneHome)
	if phoneHomeResp.Code != http.StatusOK {
		t.Fatalf("expected 200 from phone-home, got %d", phoneHomeResp.Code)
	}
	if len(device.removedKeys) != 1 || device.removedKeys[0] != "client-pub-key" {
		t.Fatalf("expected RemovePeer called for client-pub-key, got %+v", device.removedKeys)
	}
}

func TestGetClientIPFromRequestParsesForwardingHeaders(t *testing.T) {
	t.Run("xff first", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/wg-init", nil)
		req.Header.Set("X-Forwarded-For", "10.9.8.7")
		if got := getClientIPFromRequest(req); got != "10.9.8.7" {
			t.Fatalf("expected X-Forwarded-For IP, got %q", got)
		}
	})

	t.Run("forwarded fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/wg-init", nil)
		req.Header.Set("Forwarded", `for="192.0.2.44";proto=https`)
		if got := getClientIPFromRequest(req); got != "192.0.2.44" {
			t.Fatalf("expected Forwarded IP, got %q", got)
		}
	})

	t.Run("remote addr fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/wg-init", nil)
		req.RemoteAddr = "203.0.113.9:12345"
		if got := getClientIPFromRequest(req); got != "203.0.113.9" {
			t.Fatalf("expected remote addr IP, got %q", got)
		}
	})
}

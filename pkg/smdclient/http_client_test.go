// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

package smdclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientUsesComponentFallbackForIPAndMAC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/smd/hsm/v2/Inventory/EthernetInterfaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case "/apis/smd/hsm/v2/State/Components/x1000c0s0b0n0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","MAC":"aa:bb:cc:dd:ee:ff","IPAddress":"10.252.0.26"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "")

	ip, err := client.IPfromID("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("IPfromID returned error: %v", err)
	}
	if ip != "10.252.0.26" {
		t.Fatalf("expected fallback IP 10.252.0.26, got %q", ip)
	}

	mac, err := client.MACfromID("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("MACfromID returned error: %v", err)
	}
	if mac != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("expected fallback MAC aa:bb:cc:dd:ee:ff, got %q", mac)
	}
}

func TestHTTPClientReverseWireGuardLookup(t *testing.T) {
	client := NewHTTPClient("http://example.invalid", "")
	if err := client.AddWGIP("x1000c0s0b0n0", "10.100.1.25"); err != nil {
		t.Fatalf("AddWGIP returned error: %v", err)
	}

	id, err := client.IDfromWGIP("10.100.1.25")
	if err != nil {
		t.Fatalf("IDfromWGIP returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected component ID x1000c0s0b0n0, got %q", id)
	}
}

func TestResolveComponentIDPrefersWireGuardLookup(t *testing.T) {
	mock := NewMockSMDClient()
	mock.AddComponent(&Component{ID: "x1000c0s0b0n0", IP: "10.0.0.100"})
	if err := mock.AddWGIP("x1000c0s0b0n0", "10.100.1.25"); err != nil {
		t.Fatalf("AddWGIP returned error: %v", err)
	}

	id, err := ResolveComponentID(mock, "10.100.1.25")
	if err != nil {
		t.Fatalf("ResolveComponentID returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected component ID x1000c0s0b0n0, got %q", id)
	}
}

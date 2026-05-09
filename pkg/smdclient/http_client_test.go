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

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

package smdclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTTPClientUsesDynamicTokenProvider(t *testing.T) {
	const expectedToken = "dynamic-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+expectedToken {
			t.Fatalf("expected Authorization header %q, got %q", "Bearer "+expectedToken, got)
		}
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

	client := NewHTTPClientWithTokenProvider(server.URL, func() string { return expectedToken })
	_, err := client.ComponentInformation("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("ComponentInformation returned error: %v", err)
	}
}

func TestHTTPClientIDfromIPTreatsEmptyLookupResponsesAsNotFound(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "empty body", statusCode: http.StatusOK, body: ""},
		{name: "no content", statusCode: http.StatusNoContent, body: ""},
		{name: "whitespace body", statusCode: http.StatusOK, body: "   \n\t  "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/apis/smd/hsm/v2/Inventory/EthernetInterfaces" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "")
			_, err := client.IDfromIP("10.252.0.26")
			if err == nil {
				t.Fatal("expected IDfromIP to return an error")
			}
			if !strings.Contains(err.Error(), "no component found for IP 10.252.0.26") {
				t.Fatalf("expected not-found error, got %v", err)
			}
			if strings.Contains(err.Error(), "unexpected end of JSON input") {
				t.Fatalf("expected lookup error instead of JSON decode failure, got %v", err)
			}
		})
	}
}

func TestHTTPClientInjectsDynamicAuthorizationFromServiceTokenManager(t *testing.T) {
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"dynamic-token","token_type":"bearer","expires_in":3600,"refresh_token":"refresh-token","refresh_expires_in":7200,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
	}))
	defer tokensmith.Close()

	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer dynamic-token" {
			t.Fatalf("expected dynamic auth header, got %q", got)
		}
		if r.URL.Path != "/apis/smd/hsm/v2/Inventory/EthernetInterfaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"ID":"eth0","Description":"mgmt","MACAddress":"aa:bb:cc:dd:ee:ff","IPAddresses":[{"IPAddress":"10.252.0.26","Network":"HMN"}],"ComponentID":"x1000c0s0b0n0","Type":"Node"}]`))
	}))
	defer smd.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	client := NewHTTPClient(smd.URL, "static-token").WithServiceTokenManager(manager)
	id, err := client.IDfromIP("10.252.0.26")
	if err != nil {
		t.Fatalf("IDfromIP returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected component id x1000c0s0b0n0, got %q", id)
	}
}

func TestHTTPClientUsesStaticAuthorizationWhenServiceTokenManagerAbsent(t *testing.T) {
	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer static-token" {
			t.Fatalf("expected static auth header, got %q", got)
		}
		if r.URL.Path != "/apis/smd/hsm/v2/Inventory/EthernetInterfaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"ID":"eth0","Description":"mgmt","MACAddress":"aa:bb:cc:dd:ee:ff","IPAddresses":[{"IPAddress":"10.252.0.26","Network":"HMN"}],"ComponentID":"x1000c0s0b0n0","Type":"Node"}]`))
	}))
	defer smd.Close()

	client := NewHTTPClient(smd.URL, "static-token")
	id, err := client.IDfromIP("10.252.0.26")
	if err != nil {
		t.Fatalf("IDfromIP returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected component id x1000c0s0b0n0, got %q", id)
	}
}
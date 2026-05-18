// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

package smdclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare smd host defaults to api version path",
			in:   "http://smd:27779",
			want: "http://smd:27779/hsm/v2",
		},
		{
			name: "bare smd host with trailing slash defaults to api version path",
			in:   "http://smd:27779/",
			want: "http://smd:27779/hsm/v2",
		},
		{
			name: "gateway mount gets api version path appended",
			in:   "http://gw/apis/smd",
			want: "http://gw/apis/smd/hsm/v2",
		},
		{
			name: "gateway mount with trailing slash gets api version path appended",
			in:   "http://gw/apis/smd/",
			want: "http://gw/apis/smd/hsm/v2",
		},
		{
			name: "already versioned gateway url is unchanged",
			in:   "http://gw/apis/smd/hsm/v2",
			want: "http://gw/apis/smd/hsm/v2",
		},
		{
			name: "already versioned gateway url with trailing slash is normalized",
			in:   "http://gw/apis/smd/hsm/v2/",
			want: "http://gw/apis/smd/hsm/v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeBaseURL(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHTTPClientHostOnlyBaseURLUsesBareSMDPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hsm/v2/State/Components/x1000c0s0b0n0" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","IPAddress":"10.252.0.26"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "")
	component, err := client.ComponentInformation("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("ComponentInformation returned error: %v", err)
	}
	if component == nil || component.ID != "x1000c0s0b0n0" {
		t.Fatalf("unexpected component response: %+v", component)
	}
}

func TestHTTPClientUsesComponentFallbackForIPAndMAC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hsm/v2/Inventory/EthernetInterfaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case "/hsm/v2/State/Components/x1000c0s0b0n0":
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

func TestResolveComponentIDFallsBackToNaturalIPLookup(t *testing.T) {
	mock := NewMockSMDClient()
	mock.AddComponent(&Component{ID: "x1000c0s0b0n0", IP: "10.0.0.100"})

	id, err := ResolveComponentID(mock, "10.0.0.100")
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
		case "/hsm/v2/Inventory/EthernetInterfaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case "/hsm/v2/State/Components/x1000c0s0b0n0":
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
				if r.URL.Path != "/hsm/v2/Inventory/EthernetInterfaces" {
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
		if r.URL.Path != "/hsm/v2/Inventory/EthernetInterfaces" {
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
		if r.URL.Path != "/hsm/v2/Inventory/EthernetInterfaces" {
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

func TestHTTPClientGroupMembershipSupportsBothResponseShapes(t *testing.T) {
	t.Run("groupLabels", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/hsm/v2/memberships/x1000c0s0b0n0" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"groupLabels":["compute","green"]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "")
		groups, err := client.GroupMembership("x1000c0s0b0n0")
		if err != nil {
			t.Fatalf("GroupMembership returned error: %v", err)
		}
		if len(groups) != 2 || groups[0] != "compute" || groups[1] != "green" {
			t.Fatalf("unexpected groupLabels response: %+v", groups)
		}
	})

	t.Run("legacy groups", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/hsm/v2/memberships/x1000c0s0b0n0" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Groups":["compute"]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "")
		groups, err := client.GroupMembership("x1000c0s0b0n0")
		if err != nil {
			t.Fatalf("GroupMembership returned error: %v", err)
		}
		if len(groups) != 1 || groups[0] != "compute" {
			t.Fatalf("unexpected Groups response: %+v", groups)
		}
	})
}

func TestHTTPClientWGIPfromID(t *testing.T) {
	client := NewHTTPClient("http://example.invalid", "")
	if err := client.AddWGIP("x1000c0s0b0n0", "10.100.1.25"); err != nil {
		t.Fatalf("AddWGIP returned error: %v", err)
	}

	wgip, err := client.WGIPfromID("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("WGIPfromID returned error: %v", err)
	}
	if wgip != "10.100.1.25" {
		t.Fatalf("expected stored WG IP 10.100.1.25, got %q", wgip)
	}

	if _, err := client.WGIPfromID("missing"); err == nil {
		t.Fatal("expected missing WGIP lookup to error")
	}
}

func TestHTTPClientEthernetNICInfoParsesAndCaches(t *testing.T) {
	var endpointCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hsm/v2/Inventory/ComponentEndpoints/x1000c0s0b0n0" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&endpointCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"RedfishSystemInfo":{
				"EthNICInfo":[
					{"RedfishId":"1","Description":"Management","MACAddress":"aa:bb:cc:dd:ee:ff","PermanentMACAddress":"aa:bb:cc:dd:ee:ff","InterfaceEnabled":true},
					{"RedfishId":"2","Description":"HSN","MACAddress":"aa:bb:cc:dd:ee:00","PermanentMACAddress":"aa:bb:cc:dd:ee:00"}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "")
	nics, err := client.EthernetNICInfo("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("EthernetNICInfo returned error: %v", err)
	}
	if len(nics) != 2 {
		t.Fatalf("expected 2 nics, got %d", len(nics))
	}
	if !nics[0].InterfaceEnabled {
		t.Fatalf("expected nic 0 enabled")
	}
	if nics[1].InterfaceEnabled {
		t.Fatalf("expected nic 1 disabled default when missing InterfaceEnabled")
	}

	cached, err := client.EthernetNICInfo("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("EthernetNICInfo cache read returned error: %v", err)
	}
	if len(cached) != 2 {
		t.Fatalf("expected 2 cached nics, got %d", len(cached))
	}
	if got := atomic.LoadInt32(&endpointCalls); got != 1 {
		t.Fatalf("expected single endpoint call due to cache, got %d", got)
	}
}

func TestHTTPClientListComponentsPrimesIDCache(t *testing.T) {
	var listCalls int32
	var ifaceLookupCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hsm/v2/State/Components":
			atomic.AddInt32(&listCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Components":[{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","IPAddress":"10.252.0.26"}]}`))
		case "/hsm/v2/Inventory/EthernetInterfaces":
			atomic.AddInt32(&ifaceLookupCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "")
	components, err := client.ListComponents()
	if err != nil {
		t.Fatalf("ListComponents returned error: %v", err)
	}
	if len(components) != 1 || components[0].ID != "x1000c0s0b0n0" {
		t.Fatalf("unexpected ListComponents result: %+v", components)
	}

	id, err := client.IDfromIP("10.252.0.26")
	if err != nil {
		t.Fatalf("IDfromIP returned error after ListComponents: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected cached ID x1000c0s0b0n0, got %q", id)
	}

	if got := atomic.LoadInt32(&listCalls); got != 1 {
		t.Fatalf("expected one /State/Components request, got %d", got)
	}
	if got := atomic.LoadInt32(&ifaceLookupCalls); got != 0 {
		t.Fatalf("expected no EthernetInterfaces lookup due to ID cache, got %d", got)
	}
}

// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openchami/tokensmith/pkg/tokenservice"
	"github.com/spf13/viper"
)

func TestInitSMDClientRequiresURLUnlessMockFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	originalMockSMD := mockSMD
	mockSMD = false
	t.Cleanup(func() { mockSMD = originalMockSMD })

	t.Setenv("SMD_URL", "")
	t.Setenv("SMD_JWT", "")
	t.Setenv("SMD_TOKEN", "")
	t.Setenv("TOKENSMITH_URL", "")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")

	client, err := initSMDClient(context.Background())
	if err == nil {
		t.Fatal("expected initSMDClient to fail when SMD_URL is unset and --mock-smd is not enabled")
	}
	if client != nil {
		t.Fatal("expected nil client when SMD_URL is unset and --mock-smd is not enabled")
	}
	if !strings.Contains(err.Error(), "SMD_URL is required unless --mock-smd is set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitSMDClientAllowsExplicitMockFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	originalMockSMD := mockSMD
	mockSMD = true
	t.Cleanup(func() { mockSMD = originalMockSMD })

	t.Setenv("SMD_URL", "")
	t.Setenv("SMD_JWT", "")
	t.Setenv("SMD_TOKEN", "")
	t.Setenv("TOKENSMITH_URL", "")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")

	client, err := initSMDClient(context.Background())
	if err != nil {
		t.Fatalf("initSMDClient returned error: %v", err)
	}

	id, err := client.IDfromIP("10.252.0.26")
	if err != nil {
		t.Fatalf("IDfromIP returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected x1000c0s0b0n0, got %q", id)
	}
}

func TestInitSMDClientStaticModeWithoutTokenSmith(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer static-jwt" {
			t.Fatalf("expected static bearer token, got %q", got)
		}
		if r.URL.Path != "/apis/smd/hsm/v2/Inventory/EthernetInterfaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"ID":"eth0","Description":"mgmt","MACAddress":"aa:bb:cc:dd:ee:ff","IPAddresses":[{"IPAddress":"10.252.0.26","Network":"HMN"}],"ComponentID":"x1000c0s0b0n0","Type":"Node"}]`))
	}))
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("SMD_JWT", "static-jwt")
	t.Setenv("SMD_TOKEN", "")
	t.Setenv("TOKENSMITH_URL", "")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")

	client, err := initSMDClient(context.Background())
	if err != nil {
		t.Fatalf("initSMDClient returned error: %v", err)
	}

	id, err := client.IDfromIP("10.252.0.26")
	if err != nil {
		t.Fatalf("IDfromIP returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected x1000c0s0b0n0, got %q", id)
	}
}

func TestInitSMDClientDynamicModeWithTokenSmith(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	var tokenRequests int32
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if r.Form.Get("grant_type") != tokenservice.GrantTypeTokenExchange {
			t.Fatalf("expected token exchange grant, got %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("subject_token") != "bootstrap-token" {
			t.Fatalf("expected bootstrap token, got %q", r.Form.Get("subject_token"))
		}
		atomic.AddInt32(&tokenRequests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"dynamic-jwt","token_type":"bearer","expires_in":3600,"refresh_token":"refresh-jwt","refresh_expires_in":7200,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
	}))
	defer tokensmith.Close()

	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer dynamic-jwt" {
			t.Fatalf("expected dynamic bearer token, got %q", got)
		}
		if r.URL.Path != "/apis/smd/hsm/v2/Inventory/EthernetInterfaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"ID":"eth0","Description":"mgmt","MACAddress":"aa:bb:cc:dd:ee:ff","IPAddresses":[{"IPAddress":"10.252.0.26","Network":"HMN"}],"ComponentID":"x1000c0s0b0n0","Type":"Node"}]`))
	}))
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("SMD_JWT", "static-jwt")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")
	viper.Set("tokensmith_url", tokensmith.URL)
	viper.Set("tokensmith_bootstrap_token", "bootstrap-token")
	viper.Set("tokensmith_target_service", "smd")
	viper.Set("tokensmith_scopes", "metadata:read,groups:read")
	viper.Set("tokensmith_refresh_skew_sec", 300)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := initSMDClient(ctx)
	if err != nil {
		t.Fatalf("initSMDClient returned error: %v", err)
	}

	id, err := client.IDfromIP("10.252.0.26")
	if err != nil {
		t.Fatalf("IDfromIP returned error: %v", err)
	}
	if id != "x1000c0s0b0n0" {
		t.Fatalf("expected x1000c0s0b0n0, got %q", id)
	}
	if atomic.LoadInt32(&tokenRequests) == 0 {
		t.Fatal("expected at least one TokenSmith /oauth/token request")
	}
}

func TestInitSMDClientDynamicModeFailsWithoutBootstrapToken(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("SMD_URL", "http://smd.example.com")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")
	viper.Set("tokensmith_url", "http://tokensmith.example.com")
	viper.Set("tokensmith_bootstrap_token", "")

	client, err := initSMDClient(context.Background())
	if err == nil {
		t.Fatal("expected initSMDClient to fail")
	}
	if client != nil {
		t.Fatal("expected nil client on bootstrap-token misconfiguration")
	}
	if !strings.Contains(err.Error(), "bootstrap token") {
		t.Fatalf("expected bootstrap-token error, got %q", err)
	}
}

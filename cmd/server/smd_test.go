// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openchami/metadata-service/pkg/smdclient"
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
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CERT", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_KEY", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CA", "")

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
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CERT", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_KEY", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CA", "")

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
		if r.URL.Path != "/hsm/v2/Inventory/EthernetInterfaces" {
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
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CERT", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_KEY", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CA", "")

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
		if r.URL.Path != "/hsm/v2/Inventory/EthernetInterfaces" {
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
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CERT", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_KEY", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CA", "")
	viper.Set("tokensmith_url", tokensmith.URL)
	viper.Set("tokensmith_bootstrap_token", "bootstrap-token")
	viper.Set("tokensmith_target_service", "smd")
	viper.Set("tokensmith_bootstrap_policy_scopes_hint", "metadata:read,groups:read")
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

func TestInitSMDClientDynamicModeFailsWithoutCredentials(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("SMD_URL", "http://smd.example.com")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CERT", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_KEY", "")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CA", "")
	viper.Set("tokensmith_url", "http://tokensmith.example.com")
	viper.Set("tokensmith_bootstrap_token", "")

	client, err := initSMDClient(context.Background())
	if err == nil {
		t.Fatal("expected initSMDClient to fail")
	}
	if client != nil {
		t.Fatal("expected nil client on bootstrap-token misconfiguration")
	}
	if !strings.Contains(err.Error(), "requires one of") {
		t.Fatalf("expected missing dynamic credential error, got %q", err)
	}
}

func TestLoadTokenExchangeConfigFallsBackToBootstrapWhenIdentityFilesUnreadable(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("tokensmith_url", "https://tokensmith.example.com")
	viper.Set("tokensmith_bootstrap_token", "bootstrap-token")
	viper.Set("tokensmith_service_identity_cert", "/path/does/not/exist/cert.pem")
	viper.Set("tokensmith_service_identity_key", "/path/does/not/exist/key.pem")

	config, enabled, err := loadTokenExchangeConfig()
	if err != nil {
		t.Fatalf("loadTokenExchangeConfig returned error: %v", err)
	}
	if !enabled {
		t.Fatal("expected dynamic mode enabled when tokensmith_url is set")
	}
	if config.AuthMethod != smdclient.TokenAuthMethodBootstrapToken {
		t.Fatalf("expected bootstrap fallback auth method, got %q", config.AuthMethod)
	}
	if config.BootstrapToken != "bootstrap-token" {
		t.Fatalf("expected bootstrap token to be preserved, got %q", config.BootstrapToken)
	}
}

func TestLoadTokenExchangeConfigParsesScopeHint(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("tokensmith_url", "https://tokensmith.example.com")
	viper.Set("tokensmith_bootstrap_token", "bootstrap-token")
	viper.Set("tokensmith_bootstrap_policy_scopes_hint", "metadata:read, groups:read")

	config, enabled, err := loadTokenExchangeConfig()
	if err != nil {
		t.Fatalf("loadTokenExchangeConfig returned error: %v", err)
	}
	if !enabled {
		t.Fatal("expected dynamic mode enabled when tokensmith_url is set")
	}
	expected := []string{"metadata:read", "groups:read"}
	if strings.Join(config.Scopes, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected scopes %v, got %v", expected, config.Scopes)
	}
}

func TestLoadTokenExchangeConfigUsesMTLSWhenIdentityFilesReadable(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	tempCert, err := os.CreateTemp(t.TempDir(), "cert-*.pem")
	if err != nil {
		t.Fatalf("CreateTemp cert failed: %v", err)
	}
	if err := tempCert.Close(); err != nil {
		t.Fatalf("Close cert failed: %v", err)
	}
	tempKey, err := os.CreateTemp(t.TempDir(), "key-*.pem")
	if err != nil {
		t.Fatalf("CreateTemp key failed: %v", err)
	}
	if err := tempKey.Close(); err != nil {
		t.Fatalf("Close key failed: %v", err)
	}

	viper.Set("tokensmith_url", "https://tokensmith.example.com")
	viper.Set("tokensmith_service_identity_cert", tempCert.Name())
	viper.Set("tokensmith_service_identity_key", tempKey.Name())
	viper.Set("tokensmith_bootstrap_token", "bootstrap-token")

	config, enabled, err := loadTokenExchangeConfig()
	if err != nil {
		t.Fatalf("loadTokenExchangeConfig returned error: %v", err)
	}
	if !enabled {
		t.Fatal("expected dynamic mode enabled when tokensmith_url is set")
	}
	if config.AuthMethod != smdclient.TokenAuthMethodMTLSIdentity {
		t.Fatalf("expected mTLS identity auth method, got %q", config.AuthMethod)
	}
}

func TestInitSMDRuntimeWithMockReturnsWithoutBlocking(t *testing.T) {
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

	// Measure time for initSMDRuntime - should be very fast (< 100ms)
	start := time.Now()
	smdRuntime, err := initSMDRuntime()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("initSMDRuntime returned error: %v", err)
	}
	if smdRuntime.client == nil {
		t.Fatal("expected non-nil client")
	}
	if smdRuntime.startWorkers == nil {
		t.Fatal("expected non-nil startWorkers")
	}

	if elapsed > 100*time.Millisecond {
		t.Fatalf("initSMDRuntime took %v, expected < 100ms (non-blocking)", elapsed)
	}

	// Verify startWorkers also returns quickly without blocking
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start = time.Now()
	smdRuntime.startWorkers(ctx)
	elapsed = time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Fatalf("startWorkers took %v, expected < 50ms (non-blocking)", elapsed)
	}
}

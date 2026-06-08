// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchami/tokensmith/pkg/tokenservice"
	"github.com/spf13/viper"
)

func resetSMDEnv(t *testing.T) {
	t.Helper()
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
	t.Setenv("TOKENSMITH_TARGET_SERVICE", "")
	t.Setenv("TOKENSMITH_REFRESH_SKEW_SEC", "")
	t.Setenv("TOKENSMITH_SCOPE_HINT", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
}

func TestInitSMDRuntimeRequiresURLUnlessMockFlag(t *testing.T) {
	resetSMDEnv(t)

	runtime, err := initSMDRuntime()
	if err == nil {
		t.Fatal("expected initSMDRuntime to fail when SMD_URL is unset and --mock-smd is not enabled")
	}
	if runtime.client != nil {
		t.Fatal("expected nil runtime client when initSMDRuntime fails")
	}
	if got := err.Error(); !strings.Contains(got, "SMD_URL is required unless --mock-smd is set") {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestInitSMDRuntimeTokenSmithBootstrapInjectsBearer(t *testing.T) {
	resetSMDEnv(t)

	const exchangedToken = "tokensmith-service-token"

	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST to TokenSmith, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if r.Form.Get("grant_type") != tokenservice.GrantTypeTokenExchange {
			t.Fatalf("expected token exchange grant, got %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("subject_token") != "bootstrap-token" {
			t.Fatalf("expected bootstrap subject token, got %q", r.Form.Get("subject_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"` + exchangedToken + `","token_type":"bearer","expires_in":3600,"refresh_token":"refresh-token","refresh_expires_in":7200,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
	}))
	defer tokensmith.Close()

	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+exchangedToken {
			t.Fatalf("expected exchanged bearer token, got %q", got)
		}
		switch r.URL.Path {
		case "/hsm/v2/State/Components/x1000c0s0b0n0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","IPAddress":"10.252.0.26"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("TOKENSMITH_URL", tokensmith.URL)
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "bootstrap-token")

	runtime, err := initSMDRuntime()
	if err != nil {
		t.Fatalf("initSMDRuntime returned error: %v", err)
	}
	if runtime.client == nil {
		t.Fatal("expected initialized runtime client")
	}

	component, err := runtime.client.ComponentInformation("x1000c0s0b0n0")
	if err != nil {
		t.Fatalf("ComponentInformation failed: %v", err)
	}
	if component == nil || component.ID != "x1000c0s0b0n0" {
		t.Fatalf("unexpected component response: %+v", component)
	}
}

func TestInitSMDRuntimeStaticTokenFallbackWhenTokenSmithUnset(t *testing.T) {
	resetSMDEnv(t)

	const staticToken = "static-jwt"
	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+staticToken {
			t.Fatalf("expected static bearer token, got %q", got)
		}
		switch r.URL.Path {
		case "/hsm/v2/State/Components/x1000c0s0b0n0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","IPAddress":"10.252.0.26"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("SMD_JWT", staticToken)

	runtime, err := initSMDRuntime()
	if err != nil {
		t.Fatalf("initSMDRuntime returned error: %v", err)
	}
	if runtime.client == nil {
		t.Fatal("expected initialized runtime client")
	}

	if _, err := runtime.client.ComponentInformation("x1000c0s0b0n0"); err != nil {
		t.Fatalf("ComponentInformation failed: %v", err)
	}
}

func TestInitSMDRuntimeTokenSmithBootstrapFailureAbortsStartup(t *testing.T) {
	resetSMDEnv(t)

	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "exchange failed", http.StatusInternalServerError)
	}))
	defer tokensmith.Close()

	smd := httptest.NewServer(http.NotFoundHandler())
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("TOKENSMITH_URL", tokensmith.URL)
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "bootstrap-token")

	runtime, err := initSMDRuntime()
	if err == nil {
		t.Fatal("expected initSMDRuntime to fail when TokenSmith bootstrap exchange fails")
	}
	if runtime.client != nil {
		t.Fatal("expected nil runtime client on dynamic auth bootstrap failure")
	}
	if !strings.Contains(err.Error(), "/oauth/token") {
		t.Fatalf("expected TokenSmith oauth endpoint failure, got %q", err)
	}
}

func TestInitSMDRuntimeTokenSmithDynamicModeRequiresCredentials(t *testing.T) {
	resetSMDEnv(t)

	t.Setenv("SMD_URL", "http://smd.example.invalid")
	t.Setenv("TOKENSMITH_URL", "http://tokensmith.example.invalid")

	runtime, err := initSMDRuntime()
	if err == nil {
		t.Fatal("expected initSMDRuntime to fail when dynamic TokenSmith credentials are missing")
	}
	if runtime.client != nil {
		t.Fatal("expected nil runtime client on dynamic auth config error")
	}
	if !strings.Contains(err.Error(), "requires one of") {
		t.Fatalf("expected missing dynamic credentials error, got %q", err)
	}
}

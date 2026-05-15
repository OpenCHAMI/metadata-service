// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
)

func resetSMDEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SMD_URL", "")
	t.Setenv("SMD_JWT", "")
	t.Setenv("SMD_TOKEN", "")
	t.Setenv("TOKENSMITH_URL", "")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")
	t.Setenv("TOKENSMITH_TARGET_SERVICE", "")
	t.Setenv("TOKENSMITH_REFRESH_SKEW_SEC", "")
	t.Setenv("TOKENSMITH_SCOPE_HINT", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
}

func TestInitSMDRuntimeTokenSmithBootstrapInjectsBearer(t *testing.T) {
	resetSMDEnv(t)

	const exchangedToken = "tokensmith-service-token"

	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST to TokenSmith, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bootstrap-token" {
			t.Fatalf("expected bootstrap authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"` + exchangedToken + `","expires_in":3600}`))
	}))
	defer tokensmith.Close()

	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+exchangedToken {
			t.Fatalf("expected exchanged bearer token, got %q", got)
		}
		switch r.URL.Path {
		case "/apis/smd/hsm/v2/State/Components/x1000c0s0b0n0":
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

	runtime := initSMDRuntime()
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
		case "/apis/smd/hsm/v2/State/Components/x1000c0s0b0n0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","IPAddress":"10.252.0.26"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("SMD_JWT", staticToken)

	runtime := initSMDRuntime()
	if runtime.client == nil {
		t.Fatal("expected initialized runtime client")
	}

	if _, err := runtime.client.ComponentInformation("x1000c0s0b0n0"); err != nil {
		t.Fatalf("ComponentInformation failed: %v", err)
	}
}

func TestInitSMDRuntimeTokenSmithBootstrapFailureDoesNotAbort(t *testing.T) {
	resetSMDEnv(t)

	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "exchange failed", http.StatusInternalServerError)
	}))
	defer tokensmith.Close()

	const staticToken = "fallback-static-token"
	smd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+staticToken {
			t.Fatalf("expected fallback static bearer token, got %q", got)
		}
		switch r.URL.Path {
		case "/apis/smd/hsm/v2/State/Components/x1000c0s0b0n0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ID":"x1000c0s0b0n0","NID":1000,"Role":"compute","IPAddress":"10.252.0.26"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer smd.Close()

	t.Setenv("SMD_URL", smd.URL)
	t.Setenv("SMD_JWT", staticToken)
	t.Setenv("TOKENSMITH_URL", tokensmith.URL)
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "bootstrap-token")

	runtime := initSMDRuntime()
	if runtime.client == nil {
		t.Fatal("expected initialized runtime client")
	}

	if _, err := runtime.client.ComponentInformation("x1000c0s0b0n0"); err != nil {
		t.Fatalf("ComponentInformation failed in degraded mode: %v", err)
	}
}

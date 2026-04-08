// Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClient_AuthTokenProvider(t *testing.T) {
	var receivedAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		response := componentResponse{ID: "x0c0s0b0n0", NID: 1, Role: "Compute"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response) //nolint:errcheck
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "")
	client.WithAuthTokenProvider(func(context.Context) (string, error) {
		return "dynamic-token", nil
	})

	if _, err := client.ComponentInformation("x0c0s0b0n0"); err != nil {
		t.Fatalf("ComponentInformation failed: %v", err)
	}

	if receivedAuthHeader != "Bearer dynamic-token" {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer dynamic-token", receivedAuthHeader)
	}
}

func TestSMDServiceTokenManager_GetTokenAndRefresh(t *testing.T) {
	var callCount int32
	var firstForm url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("expected /oauth/token, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form body: %v", err)
		}

		if atomic.LoadInt32(&callCount) == 0 {
			firstForm = r.PostForm
		}

		count := atomic.AddInt32(&callCount, 1)
		resp := map[string]interface{}{
			"access_token":       "token-" + time.Now().Format("150405") + "-" + strings.Repeat("x", int(count)),
			"token_type":         "Bearer",
			"expires_in":         1,
			"refresh_token":      "refresh-" + time.Now().Format("150405") + "-" + strings.Repeat("r", int(count)),
			"refresh_expires_in": 600,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer server.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = server.URL
	cfg.BootstrapToken = "bootstrap-jwt"
	cfg.TargetService = "smd"
	cfg.Scopes = []string{"smd:read"}
	cfg.RefreshBefore = 900 * time.Millisecond

	manager := NewServiceTokenManager(cfg)

	token1, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("first GetToken failed: %v", err)
	}
	if token1 == "" {
		t.Fatal("expected non-empty token")
	}

	if firstForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
		t.Fatalf("expected token-exchange grant type, got %q", firstForm.Get("grant_type"))
	}
	if firstForm.Get("subject_token") != "bootstrap-jwt" {
		t.Fatalf("expected bootstrap token in subject_token, got %q", firstForm.Get("subject_token"))
	}
	if firstForm.Get("subject_token_type") != "urn:openchami:params:oauth:token-type:bootstrap-token" {
		t.Fatalf("expected bootstrap token type, got %q", firstForm.Get("subject_token_type"))
	}

	time.Sleep(450 * time.Millisecond)
	token2, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("second GetToken failed: %v", err)
	}

	if token2 == token1 {
		t.Fatal("expected refreshed token to differ from first token")
	}

	if atomic.LoadInt32(&callCount) < 2 {
		t.Fatalf("expected at least 2 token exchange calls, got %d", atomic.LoadInt32(&callCount))
	}

	stats := manager.Stats()
	if stats["refresh_success_count"].(uint64) < 2 {
		t.Fatalf("expected refresh_success_count >= 2, got %v", stats["refresh_success_count"])
	}
	if stats["refresh_failure_count"].(uint64) != 0 {
		t.Fatalf("expected refresh_failure_count == 0, got %v", stats["refresh_failure_count"])
	}
}

func TestSMDServiceTokenManager_InitializeRetriesThenSucceeds(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}

		resp := map[string]interface{}{
			"access_token":       "token-after-retry",
			"token_type":         "Bearer",
			"expires_in":         600,
			"refresh_token":      "refresh-after-retry",
			"refresh_expires_in": 86400,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer server.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = server.URL
	cfg.BootstrapToken = "bootstrap-jwt"
	cfg.BootstrapMaxAttempts = 5
	cfg.BootstrapInitialBackoff = 10 * time.Millisecond
	cfg.BootstrapMaxBackoff = 20 * time.Millisecond

	manager := NewServiceTokenManager(cfg)

	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", atomic.LoadInt32(&callCount))
	}

	stats := manager.Stats()
	if stats["refresh_failure_count"].(uint64) < 2 {
		t.Fatalf("expected refresh_failure_count >= 2, got %v", stats["refresh_failure_count"])
	}
}

func TestSMDServiceTokenManager_ErrorIncludesEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = server.URL
	cfg.BootstrapToken = "bootstrap-jwt"
	cfg.BootstrapMaxAttempts = 1

	manager := NewServiceTokenManager(cfg)
	err := manager.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected initialize to fail")
	}

	if !strings.Contains(err.Error(), server.URL+"/oauth/token") {
		t.Fatalf("expected error to include endpoint, got: %v", err)
	}
}

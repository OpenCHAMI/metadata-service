// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openchami/tokensmith/pkg/tokenservice"
)

func TestServiceTokenManagerInitializeAndGetToken(t *testing.T) {
	var requests int32
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		atomic.AddInt32(&requests, 1)
		if r.Form.Get("grant_type") != tokenservice.GrantTypeTokenExchange {
			t.Fatalf("expected bootstrap token_exchange grant, got %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("subject_token") != "bootstrap-token" {
			t.Fatalf("expected bootstrap subject token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"bearer","expires_in":3600,"refresh_token":"refresh-1","refresh_expires_in":7200,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
	}))
	defer tokensmith.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	cfg.Scopes = []string{"scope:a", "scope:b"}

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	token, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken returned error: %v", err)
	}
	if token != "access-1" {
		t.Fatalf("expected access-1 token, got %q", token)
	}

	stats := manager.Stats()
	if stats.TokenEndpoint != tokensmith.URL+"/oauth/token" {
		t.Fatalf("unexpected token endpoint %q", stats.TokenEndpoint)
	}
	if stats.TargetService != "smd" {
		t.Fatalf("expected target service smd, got %q", stats.TargetService)
	}
	if len(stats.Scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(stats.Scopes))
	}
	if stats.ClientStats.RefreshSuccesses < 1 {
		t.Fatalf("expected at least one successful exchange, got %d", stats.ClientStats.RefreshSuccesses)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected single token exchange request, got %d", got)
	}
}

func TestServiceTokenManagerRefreshTokenIfNeeded(t *testing.T) {
	grantTypes := make([]string, 0, 2)
	refreshTokens := make([]string, 0, 2)
	var requestCount int32

	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}

		count := atomic.AddInt32(&requestCount, 1)
		grantTypes = append(grantTypes, r.Form.Get("grant_type"))
		refreshTokens = append(refreshTokens, r.Form.Get("refresh_token"))

		w.Header().Set("Content-Type", "application/json")
		switch count {
		case 1:
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"bearer","expires_in":1,"refresh_token":"refresh-1","refresh_expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
		case 2:
			_, _ = w.Write([]byte(`{"access_token":"access-2","token_type":"bearer","expires_in":3600,"refresh_token":"refresh-2","refresh_expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
		default:
			t.Fatalf("unexpected request count %d", count)
		}
	}))
	defer tokensmith.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	cfg.RefreshBefore = 2 * time.Second

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if err := manager.RefreshTokenIfNeeded(context.Background()); err != nil {
		t.Fatalf("RefreshTokenIfNeeded returned error: %v", err)
	}

	token, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken returned error: %v", err)
	}
	if token != "access-2" {
		t.Fatalf("expected refreshed token access-2, got %q", token)
	}

	if len(grantTypes) < 2 {
		t.Fatalf("expected at least 2 grants, got %d", len(grantTypes))
	}
	if grantTypes[0] != tokenservice.GrantTypeTokenExchange {
		t.Fatalf("first grant should be token exchange, got %q", grantTypes[0])
	}
	if grantTypes[1] != tokenservice.GrantTypeRefreshTokenRFC8693 {
		t.Fatalf("second grant should be refresh_token, got %q", grantTypes[1])
	}
	if refreshTokens[1] != "refresh-1" {
		t.Fatalf("expected rotated refresh request to use refresh-1, got %q", refreshTokens[1])
	}
}

func TestServiceTokenManagerInitializeRetriesThenSucceeds(t *testing.T) {
	var attempts int32
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-final","token_type":"bearer","expires_in":3600,"refresh_token":"refresh-final","refresh_expires_in":7200,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
	}))
	defer tokensmith.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	cfg.BootstrapMaxAttempts = 4
	cfg.BootstrapInitialBackoff = 5 * time.Millisecond
	cfg.BootstrapMaxBackoff = 10 * time.Millisecond

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts before success, got %d", got)
	}
}

func TestServiceTokenManagerInitializeErrorIncludesOAuthTokenEndpoint(t *testing.T) {
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always failing", http.StatusInternalServerError)
	}))
	defer tokensmith.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	cfg.BootstrapMaxAttempts = 1

	manager := NewServiceTokenManager(cfg)
	err := manager.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize to fail")
	}

	if !strings.Contains(err.Error(), "/oauth/token") {
		t.Fatalf("expected error to include /oauth/token endpoint, got %q", err)
	}
	if !strings.Contains(err.Error(), tokensmith.URL) {
		t.Fatalf("expected error to include tokensmith URL, got %q", err)
	}
}

func TestServiceTokenManagerGetTokenErrorIncludesOAuthTokenEndpoint(t *testing.T) {
	tokensmithURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmithURL.String()
	cfg.BootstrapToken = "bootstrap-token"
	cfg.BootstrapMaxAttempts = 1

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err == nil {
		t.Fatal("expected Initialize to fail for unreachable endpoint")
	}

	_, err = manager.GetToken(context.Background())
	if err == nil {
		t.Fatal("expected GetToken to fail")
	}
	if !strings.Contains(err.Error(), "/oauth/token") {
		t.Fatalf("expected GetToken error to include /oauth/token, got %q", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%s/oauth/token", strings.TrimRight(tokensmithURL.String(), "/"))) {
		t.Fatalf("expected GetToken error to include token endpoint, got %q", err)
	}
}

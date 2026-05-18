// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

func TestServiceTokenManagerMTLSIdentitySessionAndRefresh(t *testing.T) {
	material := newMTLSMaterial(t)

	var sessionCalls int32
	var refreshCalls int32

	server := newTokenSmithMTLSServer(t, material, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/service-identity/session":
			atomic.AddInt32(&sessionCalls, 1)
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				t.Fatalf("expected client certificate on mTLS session request")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"mtls-access-1","token_type":"bearer","expires_in":1,"refresh_token":"mtls-refresh-1","refresh_expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
		case "/oauth/token":
			atomic.AddInt32(&refreshCalls, 1)
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm failed: %v", err)
			}
			if r.Form.Get("grant_type") != tokenservice.GrantTypeRefreshTokenRFC8693 {
				t.Fatalf("expected refresh grant type, got %q", r.Form.Get("grant_type"))
			}
			if r.Form.Get("refresh_token") != "mtls-refresh-1" {
				t.Fatalf("expected rotated refresh token, got %q", r.Form.Get("refresh_token"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"mtls-access-2","token_type":"bearer","expires_in":3600,"refresh_token":"mtls-refresh-2","refresh_expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	certPath, keyPath, caPath := writeClientIdentityFiles(t, material)

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = server.URL
	cfg.AuthMethod = TokenAuthMethodMTLSIdentity
	cfg.ServiceIdentityCert = certPath
	cfg.ServiceIdentityKey = keyPath
	cfg.ServiceIdentityCA = caPath
	cfg.RefreshBefore = 1500 * time.Millisecond
	cfg.BootstrapMaxAttempts = 1
	cfg.RefreshMaxAttempts = 1

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)

	token, err := manager.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken returned error: %v", err)
	}
	if token != "mtls-access-2" {
		t.Fatalf("expected refreshed mTLS access token mtls-access-2, got %q", token)
	}
	if atomic.LoadInt32(&sessionCalls) != 1 {
		t.Fatalf("expected one mTLS session call, got %d", sessionCalls)
	}
	if atomic.LoadInt32(&refreshCalls) != 1 {
		t.Fatalf("expected one refresh call, got %d", refreshCalls)
	}

	stats := manager.Stats()
	if stats.AuthMethod != string(TokenAuthMethodMTLSIdentity) {
		t.Fatalf("expected auth method %q, got %q", TokenAuthMethodMTLSIdentity, stats.AuthMethod)
	}
	if stats.TokenEndpoint != server.URL+"/oauth/token" {
		t.Fatalf("unexpected token endpoint %q", stats.TokenEndpoint)
	}
	if stats.SessionEndpoint != server.URL+"/service-identity/session" {
		t.Fatalf("unexpected session endpoint %q", stats.SessionEndpoint)
	}
}

func TestServiceTokenManagerMTLSIdentityUnreadableMaterialFailsAfterRetries(t *testing.T) {
	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = "https://tokensmith.example.invalid"
	cfg.AuthMethod = TokenAuthMethodMTLSIdentity
	cfg.ServiceIdentityCert = "/path/does/not/exist/client.crt"
	cfg.ServiceIdentityKey = "/path/does/not/exist/client.key"
	cfg.BootstrapMaxAttempts = 2
	cfg.BootstrapInitialBackoff = 5 * time.Millisecond
	cfg.BootstrapMaxBackoff = 10 * time.Millisecond

	manager := NewServiceTokenManager(cfg)
	err := manager.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize to fail for unreadable mTLS identity material")
	}
	if !strings.Contains(err.Error(), "failed after 2 attempts") {
		t.Fatalf("expected bounded retry exhaustion error, got %q", err)
	}
	if !strings.Contains(err.Error(), "/service-identity/session") {
		t.Fatalf("expected service identity endpoint in error, got %q", err)
	}
}

func TestServiceTokenManagerSelectsBootstrapAuthWhenNoIdentityMaterial(t *testing.T) {
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"bootstrap-access","token_type":"bearer","expires_in":3600,"refresh_token":"bootstrap-refresh","refresh_expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
	}))
	defer tokensmith.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	cfg.AuthMethod = ""

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	stats := manager.Stats()
	if stats.AuthMethod != string(TokenAuthMethodBootstrapToken) {
		t.Fatalf("expected bootstrap auth method, got %q", stats.AuthMethod)
	}
}

func TestServiceTokenManagerRefreshExhaustionMarksManagerUnhealthy(t *testing.T) {
	var requestCount int32
	tokensmith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		count := atomic.AddInt32(&requestCount, 1)
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if count == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"bearer","expires_in":1,"refresh_token":"refresh-1","refresh_expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access-token"}`))
			return
		}
		http.Error(w, "refresh failed", http.StatusBadGateway)
	}))
	defer tokensmith.Close()

	cfg := DefaultTokenExchangeConfig()
	cfg.TokenSmithURL = tokensmith.URL
	cfg.BootstrapToken = "bootstrap-token"
	cfg.RefreshBefore = 1500 * time.Millisecond
	cfg.RefreshMaxAttempts = 2
	cfg.RefreshInitialBackoff = 5 * time.Millisecond
	cfg.RefreshMaxBackoff = 10 * time.Millisecond

	manager := NewServiceTokenManager(cfg)
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)

	err := manager.RefreshTokenIfNeeded(context.Background())
	if err == nil {
		t.Fatal("expected refresh exhaustion error")
	}
	if !strings.Contains(err.Error(), "failed after 2 attempts") {
		t.Fatalf("expected bounded retry exhaustion, got %q", err)
	}
	if healthy, reason := manager.HealthStatus(); healthy {
		t.Fatal("expected manager to be unhealthy after refresh retry exhaustion")
	} else if !strings.Contains(reason, "failed after 2 attempts") {
		t.Fatalf("unexpected unhealthy reason: %q", reason)
	}

	if _, tokenErr := manager.GetToken(context.Background()); tokenErr == nil {
		t.Fatal("expected GetToken to fail closed once manager is unhealthy")
	}
}

type mtlsMaterial struct {
	caPEM         []byte
	serverCertPEM []byte
	serverKeyPEM  []byte
	clientCertPEM []byte
	clientKeyPEM  []byte
}

func newMTLSMaterial(t *testing.T) mtlsMaterial {
	t.Helper()

	caPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey CA failed: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "tokensmith-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("CreateCertificate CA failed: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("ParseCertificate CA failed: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverCertPEM, serverKeyPEM := issueLeafCert(t, caCert, caPriv, big.NewInt(2), "127.0.0.1", false)
	clientCertPEM, clientKeyPEM := issueLeafCert(t, caCert, caPriv, big.NewInt(3), "metadata-service", true)

	return mtlsMaterial{
		caPEM:         caPEM,
		serverCertPEM: serverCertPEM,
		serverKeyPEM:  serverKeyPEM,
		clientCertPEM: clientCertPEM,
		clientKeyPEM:  clientKeyPEM,
	}
}

func issueLeafCert(t *testing.T, caCert *x509.Certificate, caPriv *rsa.PrivateKey, serial *big.Int, commonName string, isClient bool) ([]byte, []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey leaf failed: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if isClient {
		leafTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		leafTemplate.DNSNames = []string{"localhost"}
		leafTemplate.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &priv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("CreateCertificate leaf failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return certPEM, keyPEM
}

func writeClientIdentityFiles(t *testing.T, material mtlsMaterial) (certPath, keyPath, caPath string) {
	t.Helper()

	tempDir := t.TempDir()
	certPath = tempDir + "/client.crt"
	keyPath = tempDir + "/client.key"
	caPath = tempDir + "/ca.crt"

	if err := os.WriteFile(certPath, material.clientCertPEM, 0o600); err != nil {
		t.Fatalf("WriteFile cert failed: %v", err)
	}
	if err := os.WriteFile(keyPath, material.clientKeyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile key failed: %v", err)
	}
	if err := os.WriteFile(caPath, material.caPEM, 0o600); err != nil {
		t.Fatalf("WriteFile ca failed: %v", err)
	}

	return certPath, keyPath, caPath
}

func newTokenSmithMTLSServer(t *testing.T, material mtlsMaterial, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	serverCert, err := tls.X509KeyPair(material.serverCertPEM, material.serverKeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair server failed: %v", err)
	}
	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(material.caPEM); !ok {
		t.Fatal("AppendCertsFromPEM failed for test CA")
	}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	return server
}

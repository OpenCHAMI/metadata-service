// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestRedactToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		show  bool
		want  string
	}{
		{name: "show token", token: "secret-token", show: true, want: "secret-token"},
		{name: "empty token", token: "", want: ""},
		{name: "short token", token: "short", want: "..."},
		{name: "prefix length token", token: "123456", want: "..."},
		{name: "long token", token: "1234567", want: "123456..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactToken(tt.token, tt.show); got != tt.want {
				t.Fatalf("RedactToken(%q, %t) = %q, want %q", tt.token, tt.show, got, tt.want)
			}
		})
	}
}

func TestAuthorizationHeaderRedactionHelpers(t *testing.T) {
	for _, key := range []string{"Authorization", "authorization", "AUTHORIZATION"} {
		if !isAuthorizationHeader(key) {
			t.Errorf("isAuthorizationHeader(%q) = false, want true", key)
		}
	}
	if isAuthorizationHeader("X-Authorization") {
		t.Error("isAuthorizationHeader(X-Authorization) = true, want false")
	}

	values := []string{"Bearer secret-token", "Basic credentials"}
	want := []string{"Bearer secret...", "Basic ..."}
	if got := redactAuthHeaderValues(values, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("redactAuthHeaderValues() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(values, []string{"Bearer secret-token", "Basic credentials"}) {
		t.Fatalf("redactAuthHeaderValues() mutated its input: %#v", values)
	}
	if got := redactAuthHeaderValues(values, true); !reflect.DeepEqual(got, values) {
		t.Fatalf("redactAuthHeaderValues(show=true) = %#v, want %#v", got, values)
	}
}

func TestWithShowTokenAndChainingPreserveClientConfiguration(t *testing.T) {
	httpClient := &http.Client{}
	logger := zerolog.Nop()
	c, err := NewClient("http://localhost:8080", httpClient, logger)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	c = c.WithVersion("v1").WithBearerToken("token-value").WithShowToken(true)

	if !c.showToken || c.version != "v1" || c.bearerToken != "token-value" || c.httpClient != httpClient {
		t.Fatalf("WithShowToken() did not preserve client configuration: %#v", c)
	}
	if !c.WithVersion("v2").showToken {
		t.Error("WithVersion() did not preserve showToken")
	}
	if !c.WithBearerToken("replacement").showToken {
		t.Error("WithBearerToken() did not preserve showToken")
	}
	if c.WithShowToken(false).showToken {
		t.Error("WithShowToken(false) did not disable token disclosure")
	}
}

func TestAuthorizationTokensInDebugLogs(t *testing.T) {
	const requestToken = "request-secret-token"
	const responseToken = "response-secret-token"

	tests := []struct {
		name string
		show bool
	}{
		{name: "redacted by default"},
		{name: "shown when enabled", show: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedAuthorization string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuthorization = r.Header.Get("Authorization")
				w.Header().Set("Authorization", "Bearer "+responseToken)
				w.Header().Set("X-Test-Header", "visible-value")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			var logs bytes.Buffer
			logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
			c, err := NewClientWithBearerToken(server.URL, requestToken, nil, logger)
			if err != nil {
				t.Fatalf("NewClientWithBearerToken() failed: %v", err)
			}
			c = c.WithShowToken(tt.show).WithVersion("v1")
			if err := c.doRequest(context.Background(), http.MethodGet, "/test", nil, nil); err != nil {
				t.Fatalf("doRequest() failed: %v", err)
			}

			if receivedAuthorization != "Bearer "+requestToken {
				t.Fatalf("wire Authorization header = %q, want full token", receivedAuthorization)
			}
			output := logs.String()
			if !strings.Contains(output, "visible-value") {
				t.Error("non-Authorization response header was not logged")
			}
			if tt.show {
				if !strings.Contains(output, requestToken) || !strings.Contains(output, responseToken) {
					t.Fatalf("full tokens missing from logs when enabled: %s", output)
				}
				return
			}
			if strings.Contains(output, requestToken) || strings.Contains(output, responseToken) {
				t.Fatalf("full token leaked into default debug logs: %s", output)
			}
			if !strings.Contains(output, "reques...") || !strings.Contains(output, "respon...") {
				t.Fatalf("redacted tokens missing from logs: %s", output)
			}
		})
	}
}

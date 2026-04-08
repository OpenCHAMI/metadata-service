// Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/openchami/tokensmith/pkg/tokenservice"
	"github.com/rs/zerolog/log"
)

// TokenExchangeConfig controls how SMD service tokens are obtained from TokenSmith.
type TokenExchangeConfig struct {
	TokenSmithURL           string
	BootstrapToken          string
	TargetService           string
	Scopes                  []string
	RequestTimeout          time.Duration
	RefreshBefore           time.Duration
	BootstrapMaxAttempts    int
	BootstrapInitialBackoff time.Duration
	BootstrapMaxBackoff     time.Duration
}

// DefaultTokenExchangeConfig returns safe defaults for TokenSmith service-token exchange.
func DefaultTokenExchangeConfig() TokenExchangeConfig {
	return TokenExchangeConfig{
		RequestTimeout:          10 * time.Second,
		RefreshBefore:           2 * time.Minute,
		BootstrapMaxAttempts:    5,
		BootstrapInitialBackoff: 1 * time.Second,
		BootstrapMaxBackoff:     15 * time.Second,
	}
}

// ServiceTokenManager exchanges bootstrap tokens for short-lived service tokens and refreshes on demand.
type ServiceTokenManager struct {
	config TokenExchangeConfig
	client *tokenservice.ServiceClient
}

// NewServiceTokenManager creates a token manager for SMD service authentication.
func NewServiceTokenManager(config TokenExchangeConfig) *ServiceTokenManager {
	defaults := DefaultTokenExchangeConfig()
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaults.RequestTimeout
	}
	if config.RefreshBefore <= 0 {
		config.RefreshBefore = defaults.RefreshBefore
	}
	if config.BootstrapMaxAttempts <= 0 {
		config.BootstrapMaxAttempts = defaults.BootstrapMaxAttempts
	}
	if config.BootstrapInitialBackoff <= 0 {
		config.BootstrapInitialBackoff = defaults.BootstrapInitialBackoff
	}
	if config.BootstrapMaxBackoff <= 0 {
		config.BootstrapMaxBackoff = defaults.BootstrapMaxBackoff
	}
	if strings.TrimSpace(config.TargetService) == "" {
		config.TargetService = "smd"
	}

	client := tokenservice.NewServiceClientWithOptions(
		strings.TrimSpace(config.TokenSmithURL),
		"metadata-service",
		"metadata-service",
		"",
		"",
		tokenservice.WithHTTPClient(&http.Client{Timeout: config.RequestTimeout}),
		tokenservice.WithBootstrapToken(config.BootstrapToken),
		tokenservice.WithTargetService(strings.TrimSpace(config.TargetService)),
		tokenservice.WithScopes(config.Scopes),
		tokenservice.WithRefreshBefore(config.RefreshBefore),
		tokenservice.WithBootstrapMaxAttempts(config.BootstrapMaxAttempts),
		tokenservice.WithBootstrapInitialBackoff(config.BootstrapInitialBackoff),
		tokenservice.WithBootstrapMaxBackoff(config.BootstrapMaxBackoff),
	)

	return &ServiceTokenManager{
		config: config,
		client: client,
	}
}

// Initialize fetches the first service token. Call this during startup to fail closed.
func (m *ServiceTokenManager) Initialize(ctx context.Context) error {
	endpoint := m.serviceTokenEndpoint()
	bootstrapPresent := strings.TrimSpace(m.config.BootstrapToken) != ""

	log.Info().
		Str("component", "smdclient").
		Str("event", "service_token_init_start").
		Str("endpoint", endpoint).
		Str("target", strings.TrimSpace(m.config.TargetService)).
		Strs("scopes", sortedScopes(m.config.Scopes)).
		Bool("bootstrap_token_present", bootstrapPresent).
		Int("max_attempts", m.config.BootstrapMaxAttempts).
		Msg("initializing SMD service token exchange")

	err := m.client.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap token exchange failed: endpoint=%s: %w", endpoint, err)
	}

	stats := m.client.Stats()
	if stats.RefreshFailures > 0 {
		log.Info().
			Str("component", "smdclient").
			Str("event", "service_token_init_retried_success").
			Uint64("attempt", stats.RefreshFailures+1).
			Msg("SMD service token initialized after retry")
	}

	return nil
}

// GetToken returns a valid bearer token, refreshing if close to expiry.
func (m *ServiceTokenManager) GetToken(ctx context.Context) (string, error) {
	if err := m.client.RefreshTokenIfNeeded(ctx); err != nil {
		return "", err
	}

	token := m.client.GetServiceToken()
	if token == nil || strings.TrimSpace(token.Token) == "" {
		return "", fmt.Errorf("service token unavailable")
	}

	return token.Token, nil
}

// StartAutoRefresh keeps tokens warm in the background. It exits when ctx is canceled.
func (m *ServiceTokenManager) StartAutoRefresh(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, m.config.RequestTimeout)
			err := m.client.RefreshTokenIfNeeded(refreshCtx)
			cancel()
			if err != nil {
				stats := m.Stats()
				log.Warn().
					Str("component", "smdclient").
					Str("event", "service_token_auto_refresh_failed").
					Err(err).
					Interface("stats", stats).
					Msg("failed to refresh SMD service token")
			}
		}
	}
}

// Stats returns token refresh counters and latest refresh/error state for diagnostics.
func (m *ServiceTokenManager) Stats() map[string]interface{} {
	clientStats := m.client.Stats()

	stats := map[string]interface{}{
		"refresh_success_count": clientStats.RefreshSuccesses,
		"refresh_failure_count": clientStats.RefreshFailures,
		"last_error":            clientStats.LastError,
		"last_refresh_at":       "",
		"last_success_at":       "",
	}
	if !clientStats.LastRefresh.IsZero() {
		stats["last_refresh_at"] = clientStats.LastRefresh.UTC().Format(time.RFC3339)
	}
	if !clientStats.LastSuccess.IsZero() {
		stats["last_success_at"] = clientStats.LastSuccess.UTC().Format(time.RFC3339)
	}

	return stats
}

// GetServiceToken exposes the current shared client token for callers that want direct access.
func (m *ServiceTokenManager) GetServiceToken() *tokenservice.ServiceToken {
	return m.client.GetServiceToken()
}

// RefreshTokenIfNeeded refreshes the shared service token if it is missing or near expiry.
func (m *ServiceTokenManager) RefreshTokenIfNeeded(ctx context.Context) error {
	return m.client.RefreshTokenIfNeeded(ctx)
}

func (m *ServiceTokenManager) serviceTokenEndpoint() string {
	return strings.TrimRight(m.config.TokenSmithURL, "/") + "/oauth/token"
}

func sortedScopes(scopes []string) []string {
	filtered := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			filtered = append(filtered, scope)
		}
	}
	sort.Strings(filtered)
	return filtered
}

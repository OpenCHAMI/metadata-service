// Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

type serviceTokenRequest struct {
	BootstrapToken string   `json:"bootstrap_token"`
	TargetService  string   `json:"target_service,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
}

type serviceTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ServiceTokenManager exchanges bootstrap tokens for short-lived service tokens and refreshes on demand.
type ServiceTokenManager struct {
	config     TokenExchangeConfig
	httpClient *http.Client

	mu        sync.RWMutex
	token     string
	expiresAt time.Time

	refreshSuccessCount uint64
	refreshFailureCount uint64

	statusMu      sync.RWMutex
	lastError     string
	lastRefreshAt time.Time
	lastSuccessAt time.Time
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

	return &ServiceTokenManager{
		config:     config,
		httpClient: &http.Client{Timeout: config.RequestTimeout},
	}
}

// Initialize fetches the first service token. Call this during startup to fail closed.
func (m *ServiceTokenManager) Initialize(ctx context.Context) error {
	var lastErr error
	backoff := m.config.BootstrapInitialBackoff
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

	for attempt := 1; attempt <= m.config.BootstrapMaxAttempts; attempt++ {
		_, err := m.GetToken(ctx)
		if err == nil {
			if attempt > 1 {
				log.Info().
					Str("component", "smdclient").
					Str("event", "service_token_init_retried_success").
					Int("attempt", attempt).
					Msg("SMD service token initialized after retry")
			}
			return nil
		}

		lastErr = err
		if attempt == m.config.BootstrapMaxAttempts {
			break
		}

		log.Warn().
			Str("component", "smdclient").
			Str("event", "service_token_init_retry_failed").
			Int("attempt", attempt).
			Int("max_attempts", m.config.BootstrapMaxAttempts).
			Str("endpoint", endpoint).
			Str("target", strings.TrimSpace(m.config.TargetService)).
			Strs("scopes", sortedScopes(m.config.Scopes)).
			Bool("bootstrap_token_present", bootstrapPresent).
			Err(err).
			Msg("bootstrap token exchange attempt failed")

		wait := backoff
		if wait > m.config.BootstrapMaxBackoff {
			wait = m.config.BootstrapMaxBackoff
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("bootstrap token exchange canceled: %w", ctx.Err())
		case <-time.After(wait):
		}

		if backoff < m.config.BootstrapMaxBackoff {
			backoff *= 2
			if backoff > m.config.BootstrapMaxBackoff {
				backoff = m.config.BootstrapMaxBackoff
			}
		}
	}

	return fmt.Errorf("bootstrap token exchange failed after %d attempts: %w", m.config.BootstrapMaxAttempts, lastErr)
}

// GetToken returns a valid bearer token, refreshing if close to expiry.
func (m *ServiceTokenManager) GetToken(ctx context.Context) (string, error) {
	if token, ok := m.readValidToken(); ok {
		return token, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hasValidTokenLocked() {
		return m.token, nil
	}

	if err := m.refreshTokenLocked(ctx); err != nil {
		return "", err
	}

	return m.token, nil
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
			_, err := m.GetToken(refreshCtx)
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
	m.statusMu.RLock()
	lastError := m.lastError
	lastRefreshAt := m.lastRefreshAt
	lastSuccessAt := m.lastSuccessAt
	m.statusMu.RUnlock()

	stats := map[string]interface{}{
		"refresh_success_count": atomic.LoadUint64(&m.refreshSuccessCount),
		"refresh_failure_count": atomic.LoadUint64(&m.refreshFailureCount),
		"last_error":            lastError,
		"last_refresh_at":       "",
		"last_success_at":       "",
	}
	if !lastRefreshAt.IsZero() {
		stats["last_refresh_at"] = lastRefreshAt.UTC().Format(time.RFC3339)
	}
	if !lastSuccessAt.IsZero() {
		stats["last_success_at"] = lastSuccessAt.UTC().Format(time.RFC3339)
	}

	return stats
}

func (m *ServiceTokenManager) readValidToken() (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.hasValidTokenLocked() {
		return "", false
	}

	return m.token, true
}

func (m *ServiceTokenManager) hasValidTokenLocked() bool {
	if strings.TrimSpace(m.token) == "" || m.expiresAt.IsZero() {
		return false
	}

	return time.Until(m.expiresAt) > m.config.RefreshBefore
}

func (m *ServiceTokenManager) refreshTokenLocked(ctx context.Context) error {
	requestBody, err := json.Marshal(serviceTokenRequest{
		BootstrapToken: m.config.BootstrapToken,
		TargetService:  strings.TrimSpace(m.config.TargetService),
		Scopes:         m.config.Scopes,
	})
	if err != nil {
		return fmt.Errorf("failed to encode service token request: %w", err)
	}

	url := m.serviceTokenEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create service token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		m.recordRefreshFailure(fmt.Errorf("failed to request service token: %w", err))
		return fmt.Errorf("failed to request service token: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("service token request failed: endpoint=%s status=%d body=%s", url, resp.StatusCode, strings.TrimSpace(string(body)))
		m.recordRefreshFailure(err)
		return err
	}

	var tokenResp serviceTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		m.recordRefreshFailure(fmt.Errorf("failed to decode service token response: %w", err))
		return fmt.Errorf("failed to decode service token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.Token) == "" {
		err := fmt.Errorf("service token response did not contain a token")
		m.recordRefreshFailure(err)
		return err
	}
	if tokenResp.ExpiresAt.IsZero() {
		err := fmt.Errorf("service token response did not contain expires_at")
		m.recordRefreshFailure(err)
		return err
	}

	m.token = tokenResp.Token
	m.expiresAt = tokenResp.ExpiresAt
	m.recordRefreshSuccess()
	log.Info().
		Str("component", "smdclient").
		Str("event", "service_token_refreshed").
		Dur("expires_in", time.Until(m.expiresAt).Round(time.Second)).
		Msg("refreshed SMD service token")

	return nil
}

func (m *ServiceTokenManager) recordRefreshSuccess() {
	atomic.AddUint64(&m.refreshSuccessCount, 1)
	m.statusMu.Lock()
	m.lastRefreshAt = time.Now().UTC()
	m.lastSuccessAt = m.lastRefreshAt
	m.lastError = ""
	m.statusMu.Unlock()
}

func (m *ServiceTokenManager) recordRefreshFailure(err error) {
	atomic.AddUint64(&m.refreshFailureCount, 1)
	m.statusMu.Lock()
	m.lastRefreshAt = time.Now().UTC()
	m.lastError = err.Error()
	m.statusMu.Unlock()
}

func (m *ServiceTokenManager) serviceTokenEndpoint() string {
	return strings.TrimRight(m.config.TokenSmithURL, "/") + "/service/token"
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

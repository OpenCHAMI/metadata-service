// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/openchami/tokensmith/pkg/tokenservice"
	"github.com/rs/zerolog/log"
)

const (
	defaultServiceIdentity = "metadata-service"
	defaultTargetService   = "smd"
)

// TokenAuthMethod identifies which TokenSmith dynamic auth flow is active.
type TokenAuthMethod string

const (
	TokenAuthMethodBootstrapToken TokenAuthMethod = "bootstrap_token"
	TokenAuthMethodMTLSIdentity   TokenAuthMethod = "mtls_identity"
)

// TokenExchangeConfig controls dynamic TokenSmith-backed service-token exchange.
type TokenExchangeConfig struct {
	TokenSmithURL  string
	BootstrapToken string
	TargetService  string
	Scopes         []string
	AuthMethod     TokenAuthMethod

	ServiceIdentityCert string
	ServiceIdentityKey  string
	ServiceIdentityCA   string

	RefreshBefore           time.Duration
	AutoRefreshInterval     time.Duration
	BootstrapMaxAttempts    int
	BootstrapInitialBackoff time.Duration
	BootstrapMaxBackoff     time.Duration
	RefreshMaxAttempts      int
	RefreshInitialBackoff   time.Duration
	RefreshMaxBackoff       time.Duration

	ServiceName string
	ServiceID   string
	InstanceID  string
	ClusterID   string
}

// DefaultTokenExchangeConfig returns sane defaults for service-token lifecycle handling.
func DefaultTokenExchangeConfig() TokenExchangeConfig {
	return TokenExchangeConfig{
		TargetService:           defaultTargetService,
		RefreshBefore:           5 * time.Minute,
		AutoRefreshInterval:     time.Minute,
		BootstrapMaxAttempts:    5,
		BootstrapInitialBackoff: time.Second,
		BootstrapMaxBackoff:     15 * time.Second,
		RefreshMaxAttempts:      5,
		RefreshInitialBackoff:   time.Second,
		RefreshMaxBackoff:       15 * time.Second,
		ServiceName:             defaultServiceIdentity,
		ServiceID:               defaultServiceIdentity,
	}
}

// ServiceTokenManagerStats exposes service-token diagnostics for logging and health checks.
type ServiceTokenManagerStats struct {
	AuthMethod      string
	TokenEndpoint   string
	SessionEndpoint string
	TargetService   string
	Scopes          []string
	Unhealthy       bool
	UnhealthyReason string
	ClientStats     tokenservice.ServiceClientStats
}

// ServiceTokenManager wraps TokenSmith service-token exchange and refresh behavior.
type ServiceTokenManager struct {
	config             TokenExchangeConfig
	client             *tokenservice.ServiceClient
	httpClient         *http.Client
	tokenEndpoint      string
	sessionEndpoint    string
	targetService      string
	scopes             []string
	authMethod         TokenAuthMethod
	refreshMaxAttempts int
	refreshBackoff     time.Duration
	refreshMaxBackoff  time.Duration

	mu            sync.RWMutex
	token         *tokenservice.ServiceToken
	refreshToken  string
	refreshExpiry time.Time
	unhealthy     bool
	unhealthyErr  string
}

// NewServiceTokenManager builds a manager around the TokenSmith service client.
func NewServiceTokenManager(config TokenExchangeConfig) *ServiceTokenManager {
	normalized := normalizeTokenExchangeConfig(config)
	baseURL := strings.TrimRight(strings.TrimSpace(normalized.TokenSmithURL), "/")

	manager := &ServiceTokenManager{
		config:             normalized,
		tokenEndpoint:      baseURL + "/oauth/token",
		sessionEndpoint:    baseURL + "/service-identity/session",
		targetService:      normalized.TargetService,
		scopes:             append([]string(nil), normalized.Scopes...),
		authMethod:         normalized.AuthMethod,
		refreshMaxAttempts: normalized.RefreshMaxAttempts,
		refreshBackoff:     normalized.RefreshInitialBackoff,
		refreshMaxBackoff:  normalized.RefreshMaxBackoff,
	}

	if normalized.AuthMethod == TokenAuthMethodBootstrapToken {
		opts := []tokenservice.ServiceClientOption{
			tokenservice.WithTargetService(normalized.TargetService),
			tokenservice.WithRefreshBefore(normalized.RefreshBefore),
			tokenservice.WithAutoRefreshInterval(normalized.AutoRefreshInterval),
			tokenservice.WithBootstrapMaxAttempts(normalized.BootstrapMaxAttempts),
			tokenservice.WithBootstrapInitialBackoff(normalized.BootstrapInitialBackoff),
			tokenservice.WithBootstrapMaxBackoff(normalized.BootstrapMaxBackoff),
		}
		if normalized.BootstrapToken != "" {
			opts = append(opts, tokenservice.WithBootstrapToken(normalized.BootstrapToken))
		}

		manager.client = tokenservice.NewServiceClientWithOptions(
			normalized.TokenSmithURL,
			normalized.ServiceName,
			normalized.ServiceID,
			normalized.InstanceID,
			normalized.ClusterID,
			opts...,
		)
	}

	return manager
}

func normalizeTokenExchangeConfig(config TokenExchangeConfig) TokenExchangeConfig {
	defaults := DefaultTokenExchangeConfig()

	if strings.TrimSpace(config.TokenSmithURL) != "" {
		defaults.TokenSmithURL = strings.TrimSpace(config.TokenSmithURL)
	}
	if strings.TrimSpace(config.BootstrapToken) != "" {
		defaults.BootstrapToken = strings.TrimSpace(config.BootstrapToken)
	}
	if strings.TrimSpace(config.TargetService) != "" {
		defaults.TargetService = strings.TrimSpace(config.TargetService)
	}
	if strings.TrimSpace(string(config.AuthMethod)) != "" {
		defaults.AuthMethod = TokenAuthMethod(strings.TrimSpace(string(config.AuthMethod)))
	}
	if config.Scopes != nil {
		defaults.Scopes = append([]string(nil), config.Scopes...)
	}
	if strings.TrimSpace(config.ServiceIdentityCert) != "" {
		defaults.ServiceIdentityCert = strings.TrimSpace(config.ServiceIdentityCert)
	}
	if strings.TrimSpace(config.ServiceIdentityKey) != "" {
		defaults.ServiceIdentityKey = strings.TrimSpace(config.ServiceIdentityKey)
	}
	if strings.TrimSpace(config.ServiceIdentityCA) != "" {
		defaults.ServiceIdentityCA = strings.TrimSpace(config.ServiceIdentityCA)
	}
	if config.RefreshBefore > 0 {
		defaults.RefreshBefore = config.RefreshBefore
	}
	if config.AutoRefreshInterval > 0 {
		defaults.AutoRefreshInterval = config.AutoRefreshInterval
	}
	if config.BootstrapMaxAttempts > 0 {
		defaults.BootstrapMaxAttempts = config.BootstrapMaxAttempts
	}
	if config.BootstrapInitialBackoff > 0 {
		defaults.BootstrapInitialBackoff = config.BootstrapInitialBackoff
	}
	if config.BootstrapMaxBackoff > 0 {
		defaults.BootstrapMaxBackoff = config.BootstrapMaxBackoff
	}
	if config.RefreshMaxAttempts > 0 {
		defaults.RefreshMaxAttempts = config.RefreshMaxAttempts
	}
	if config.RefreshInitialBackoff > 0 {
		defaults.RefreshInitialBackoff = config.RefreshInitialBackoff
	}
	if config.RefreshMaxBackoff > 0 {
		defaults.RefreshMaxBackoff = config.RefreshMaxBackoff
	}
	if strings.TrimSpace(config.ServiceName) != "" {
		defaults.ServiceName = strings.TrimSpace(config.ServiceName)
	}
	if strings.TrimSpace(config.ServiceID) != "" {
		defaults.ServiceID = strings.TrimSpace(config.ServiceID)
	}
	if strings.TrimSpace(config.InstanceID) != "" {
		defaults.InstanceID = strings.TrimSpace(config.InstanceID)
	}
	if strings.TrimSpace(config.ClusterID) != "" {
		defaults.ClusterID = strings.TrimSpace(config.ClusterID)
	}
	if defaults.RefreshMaxAttempts <= 0 {
		defaults.RefreshMaxAttempts = defaults.BootstrapMaxAttempts
	}
	if defaults.RefreshInitialBackoff <= 0 {
		defaults.RefreshInitialBackoff = defaults.BootstrapInitialBackoff
	}
	if defaults.RefreshMaxBackoff <= 0 {
		defaults.RefreshMaxBackoff = defaults.BootstrapMaxBackoff
	}
	if defaults.AuthMethod == "" {
		if defaults.ServiceIdentityCert != "" && defaults.ServiceIdentityKey != "" {
			defaults.AuthMethod = TokenAuthMethodMTLSIdentity
		} else {
			defaults.AuthMethod = TokenAuthMethodBootstrapToken
		}
	}

	return defaults
}

// Initialize performs the startup bootstrap exchange and retries per config.
func (m *ServiceTokenManager) Initialize(ctx context.Context) error {
	startTime := time.Now()
	log.Debug().
		Str("auth_method", string(m.authMethod)).
		Str("endpoint", m.tokenEndpoint).
		Msg("Token manager initialization starting")

	if err := m.errIfUnhealthy(); err != nil {
		return err
	}

	switch m.authMethod {
	case TokenAuthMethodBootstrapToken:
		if m.client == nil {
			log.Error().
				Str("endpoint", m.tokenEndpoint).
				Msg("Bootstrap token exchange failed: service client not configured")
			return fmt.Errorf("bootstrap token exchange via %s failed: service client not configured", m.tokenEndpoint)
		}
		log.Debug().
			Str("endpoint", m.tokenEndpoint).
			Msg("Initializing bootstrap token exchange")
		if err := m.client.Initialize(ctx); err != nil {
			log.Error().
				Err(err).
				Str("endpoint", m.tokenEndpoint).
				Dur("duration_ms", time.Since(startTime)).
				Msg("Bootstrap token exchange failed")
			return m.wrapEndpointError("bootstrap token exchange", m.tokenEndpoint, err)
		}
		log.Info().
			Str("endpoint", m.tokenEndpoint).
			Dur("duration_ms", time.Since(startTime)).
			Msg("Bootstrap token exchange completed successfully")
		return nil
	case TokenAuthMethodMTLSIdentity:
		log.Debug().
			Str("endpoint", m.sessionEndpoint).
			Msg("Initializing mTLS service identity session")
		err := m.withRetries(
			ctx,
			"service identity session exchange",
			m.sessionEndpoint,
			m.config.BootstrapMaxAttempts,
			m.config.BootstrapInitialBackoff,
			m.config.BootstrapMaxBackoff,
			false,
			func(opCtx context.Context) error {
				return m.initializeMTLS(opCtx)
			},
		)
		if err != nil {
			log.Error().
				Err(err).
				Str("endpoint", m.sessionEndpoint).
				Dur("duration_ms", time.Since(startTime)).
				Msg("mTLS service identity session failed")
			return err
		}
		log.Info().
			Str("endpoint", m.sessionEndpoint).
			Dur("duration_ms", time.Since(startTime)).
			Msg("mTLS service identity session completed successfully")
		return nil
	default:
		log.Error().
			Str("auth_method", string(m.authMethod)).
			Msg("Unsupported token auth method")
		return fmt.Errorf("unsupported token auth method %q", m.authMethod)
	}
}

// GetToken returns a current access token, refreshing when the configured skew is reached.
func (m *ServiceTokenManager) GetToken(ctx context.Context) (string, error) {
	if err := m.errIfUnhealthy(); err != nil {
		log.Warn().
			Err(err).
			Str("auth_method", string(m.authMethod)).
			Msg("GetToken called on unhealthy token manager")
		return "", err
	}
	if err := m.RefreshTokenIfNeeded(ctx); err != nil {
		log.Warn().
			Err(err).
			Str("auth_method", string(m.authMethod)).
			Msg("Token refresh check failed")
		return "", err
	}

	switch m.authMethod {
	case TokenAuthMethodBootstrapToken:
		if m.client == nil {
			log.Error().
				Str("endpoint", m.tokenEndpoint).
				Msg("Service token unavailable: client not configured")
			return "", fmt.Errorf("service token unavailable from %s", m.tokenEndpoint)
		}
		token := m.client.GetServiceToken()
		if token == nil || strings.TrimSpace(token.Token) == "" {
			log.Error().
				Str("endpoint", m.tokenEndpoint).
				Msg("Service token unavailable: empty token from client")
			return "", fmt.Errorf("service token unavailable from %s", m.tokenEndpoint)
		}
		log.Debug().
			Str("endpoint", m.tokenEndpoint).
			Time("expires_at", token.ExpiresAt).
			Dur("time_until_expiry", time.Until(token.ExpiresAt)).
			Msg("Service token retrieved successfully")
		return strings.TrimSpace(token.Token), nil
	case TokenAuthMethodMTLSIdentity:
		m.mu.RLock()
		token := m.token
		m.mu.RUnlock()
		if token == nil || strings.TrimSpace(token.Token) == "" {
			log.Error().
				Str("endpoint", m.sessionEndpoint).
				Msg("Service token unavailable: empty mTLS token")
			return "", fmt.Errorf("service token unavailable from %s", m.sessionEndpoint)
		}
		log.Debug().
			Str("endpoint", m.sessionEndpoint).
			Time("expires_at", token.ExpiresAt).
			Dur("time_until_expiry", time.Until(token.ExpiresAt)).
			Msg("mTLS service token retrieved successfully")
		return strings.TrimSpace(token.Token), nil
	default:
		log.Error().
			Str("auth_method", string(m.authMethod)).
			Msg("Unsupported token auth method in GetToken")
		return "", fmt.Errorf("unsupported token auth method %q", m.authMethod)
	}
}

// RefreshTokenIfNeeded refreshes the current token if it is near expiry.
func (m *ServiceTokenManager) RefreshTokenIfNeeded(ctx context.Context) error {
	if err := m.errIfUnhealthy(); err != nil {
		return err
	}

	switch m.authMethod {
	case TokenAuthMethodBootstrapToken:
		err := m.withRetries(
			ctx,
			"service token refresh",
			m.tokenEndpoint,
			m.refreshMaxAttempts,
			m.refreshBackoff,
			m.refreshMaxBackoff,
			true,
			func(opCtx context.Context) error {
				if m.client == nil {
					log.Error().Msg("Token refresh failed: service client not configured")
					return fmt.Errorf("service client not configured")
				}
				if err := m.client.RefreshTokenIfNeeded(opCtx); err != nil {
					log.Warn().
						Err(err).
						Str("endpoint", m.tokenEndpoint).
						Msg("Token refresh attempt failed")
					return m.wrapEndpointError("service token refresh", m.tokenEndpoint, err)
				}
				log.Debug().
					Str("endpoint", m.tokenEndpoint).
					Msg("Token refresh completed successfully")
				return nil
			},
		)
		if err != nil {
			log.Error().
				Err(err).
				Str("endpoint", m.tokenEndpoint).
				Msg("Token refresh failed after all retry attempts")
			return err
		}
		return nil
	case TokenAuthMethodMTLSIdentity:
		needsRefresh, err := m.mtlsNeedsRefresh()
		if err != nil {
			log.Error().
				Err(err).
				Msg("Failed to check if mTLS token needs refresh")
			return m.markUnhealthy(err)
		}
		if !needsRefresh {
			log.Debug().Msg("mTLS token does not need refresh yet")
			return nil
		}
		log.Debug().Msg("mTLS token needs refresh, starting refresh process")
		err = m.withRetries(
			ctx,
			"service token refresh",
			m.tokenEndpoint,
			m.refreshMaxAttempts,
			m.refreshBackoff,
			m.refreshMaxBackoff,
			true,
			func(opCtx context.Context) error {
				return m.refreshMTLS(opCtx)
			},
		)
		if err != nil {
			log.Error().
				Err(err).
				Str("endpoint", m.tokenEndpoint).
				Msg("mTLS token refresh failed after all retry attempts")
			return err
		}
		log.Info().
			Str("endpoint", m.tokenEndpoint).
			Msg("mTLS token refresh completed successfully")
		return nil
	default:
		log.Error().
			Str("auth_method", string(m.authMethod)).
			Msg("Unsupported token auth method in RefreshTokenIfNeeded")
		return fmt.Errorf("unsupported token auth method %q", m.authMethod)
	}
}

// StartAutoRefresh launches the periodic token refresh worker in a background goroutine.
// The worker runs until the context is cancelled or the manager becomes unhealthy.
// This method returns immediately without blocking.
func (m *ServiceTokenManager) StartAutoRefresh(ctx context.Context) {
	go m.runAutoRefresh(ctx)
}

// runAutoRefresh runs periodic refresh checks until context cancellation.
func (m *ServiceTokenManager) runAutoRefresh(ctx context.Context) {
	interval := m.config.AutoRefreshInterval
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.RefreshTokenIfNeeded(ctx); err != nil {
				if m.isUnhealthy() {
					return
				}
			}
		}
	}
}

// Stats returns manager-level and TokenSmith client diagnostics.
func (m *ServiceTokenManager) Stats() ServiceTokenManagerStats {
	unhealthy, unhealthyReason := m.healthStatus()
	clientStats := tokenservice.ServiceClientStats{}
	if m.client != nil {
		clientStats = m.client.Stats()
	}

	return ServiceTokenManagerStats{
		AuthMethod:      string(m.authMethod),
		TokenEndpoint:   m.tokenEndpoint,
		SessionEndpoint: m.sessionEndpoint,
		TargetService:   m.targetService,
		Scopes:          append([]string(nil), m.scopes...),
		Unhealthy:       unhealthy,
		UnhealthyReason: unhealthyReason,
		ClientStats:     clientStats,
	}
}

// HealthStatus reports whether the manager can still provide dynamic tokens.
func (m *ServiceTokenManager) HealthStatus() (bool, string) {
	unhealthy, reason := m.healthStatus()
	if unhealthy {
		return false, reason
	}
	return true, ""
}

func (m *ServiceTokenManager) initializeMTLS(ctx context.Context) error {
	client, err := m.ensureMTLSHTTPClient()
	if err != nil {
		return err
	}

	reqBody := map[string]any{
		"service_name":   strings.TrimSpace(m.config.ServiceName),
		"service_id":     strings.TrimSpace(m.config.ServiceID),
		"instance_id":    strings.TrimSpace(m.config.InstanceID),
		"cluster_id":     strings.TrimSpace(m.config.ClusterID),
		"target_service": strings.TrimSpace(m.config.TargetService),
	}
	if len(m.scopes) > 0 {
		reqBody["scopes"] = append([]string(nil), m.scopes...)
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to encode mTLS service identity request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.sessionEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create mTLS service identity request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mTLS service identity request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	now := time.Now().UTC()
	oauthResp, err := decodeOAuthTokenResponse(resp)
	if err != nil {
		return err
	}
	if err := m.updateMTLSTokens(now, oauthResp); err != nil {
		return err
	}
	return nil
}

func (m *ServiceTokenManager) refreshMTLS(ctx context.Context) error {
	client, err := m.ensureMTLSHTTPClient()
	if err != nil {
		return err
	}

	m.mu.RLock()
	refreshToken := strings.TrimSpace(m.refreshToken)
	refreshExpiry := m.refreshExpiry
	m.mu.RUnlock()

	if refreshToken == "" {
		return fmt.Errorf("missing refresh token")
	}
	if !refreshExpiry.IsZero() && time.Now().After(refreshExpiry) {
		return fmt.Errorf("refresh token expired")
	}

	form := url.Values{}
	form.Set("grant_type", tokenservice.GrantTypeRefreshTokenRFC8693)
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("service token refresh request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	now := time.Now().UTC()
	oauthResp, err := decodeOAuthTokenResponse(resp)
	if err != nil {
		return err
	}
	if err := m.updateMTLSTokens(now, oauthResp); err != nil {
		return err
	}
	return nil
}

func (m *ServiceTokenManager) mtlsNeedsRefresh() (bool, error) {
	m.mu.RLock()
	token := m.token
	m.mu.RUnlock()

	if token == nil {
		return true, nil
	}
	if token.ExpiresAt.IsZero() {
		return true, nil
	}
	if time.Until(token.ExpiresAt) < m.config.RefreshBefore {
		return true, nil
	}
	return false, nil
}

func (m *ServiceTokenManager) updateMTLSTokens(now time.Time, oauthResp tokenservice.OAuthTokenResponse) error {
	if strings.TrimSpace(oauthResp.AccessToken) == "" || oauthResp.ExpiresIn <= 0 {
		return fmt.Errorf("failed to decode token response: missing access_token or expires_in")
	}
	if strings.TrimSpace(oauthResp.RefreshToken) == "" || oauthResp.RefreshExpiresIn <= 0 {
		return fmt.Errorf("failed to decode token response: missing refresh_token or refresh_expires_in")
	}

	m.mu.Lock()
	m.token = &tokenservice.ServiceToken{
		Token:     strings.TrimSpace(oauthResp.AccessToken),
		ExpiresAt: now.Add(time.Duration(oauthResp.ExpiresIn) * time.Second),
	}
	m.refreshToken = strings.TrimSpace(oauthResp.RefreshToken)
	m.refreshExpiry = now.Add(time.Duration(oauthResp.RefreshExpiresIn) * time.Second)
	m.mu.Unlock()
	return nil
}

func decodeOAuthTokenResponse(resp *http.Response) (tokenservice.OAuthTokenResponse, error) {
	var oauthResp tokenservice.OAuthTokenResponse
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return oauthResp, fmt.Errorf("TokenSmith endpoint returned status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&oauthResp); err != nil {
		return oauthResp, fmt.Errorf("failed to decode token response: %w", err)
	}
	return oauthResp, nil
}

func (m *ServiceTokenManager) ensureMTLSHTTPClient() (*http.Client, error) {
	m.mu.RLock()
	client := m.httpClient
	m.mu.RUnlock()
	if client != nil {
		return client, nil
	}

	certPath := strings.TrimSpace(m.config.ServiceIdentityCert)
	keyPath := strings.TrimSpace(m.config.ServiceIdentityKey)
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("mTLS service identity requires cert and key paths")
	}

	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("mTLS cert/key load failed: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
	}

	if caPath := strings.TrimSpace(m.config.ServiceIdentityCA); caPath != "" {
		caPEM, readErr := os.ReadFile(caPath)
		if readErr != nil {
			return nil, fmt.Errorf("mTLS CA read failed for %s: %w", caPath, readErr)
		}
		rootCAs := x509.NewCertPool()
		if ok := rootCAs.AppendCertsFromPEM(caPEM); !ok {
			return nil, fmt.Errorf("mTLS CA parse failed for %s", caPath)
		}
		tlsConfig.RootCAs = rootCAs
	}

	client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	m.mu.Lock()
	if m.httpClient == nil {
		m.httpClient = client
	} else {
		client = m.httpClient
	}
	m.mu.Unlock()

	return client, nil
}

func (m *ServiceTokenManager) withRetries(
	ctx context.Context,
	action string,
	endpoint string,
	maxAttempts int,
	initialBackoff time.Duration,
	maxBackoff time.Duration,
	failClosedOnExhaustion bool,
	fn func(context.Context) error,
) error {
	attempts := maxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	backoff := initialBackoff
	if backoff <= 0 {
		backoff = time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := fn(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt == attempts {
			break
		}

		wait := backoff
		if maxBackoff > 0 && wait > maxBackoff {
			wait = maxBackoff
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}

		backoff *= 2
		if maxBackoff > 0 && backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	wrapped := fmt.Errorf("%s via %s failed after %d attempts: %w", action, endpoint, attempts, lastErr)
	if failClosedOnExhaustion {
		return m.markUnhealthy(wrapped)
	}
	return wrapped
}

func (m *ServiceTokenManager) wrapEndpointError(action string, endpoint string, err error) error {
	return fmt.Errorf("%s via %s failed: %w", action, endpoint, err)
}

func (m *ServiceTokenManager) markUnhealthy(err error) error {
	if err == nil {
		return nil
	}
	m.mu.Lock()
	m.unhealthy = true
	m.unhealthyErr = err.Error()
	m.mu.Unlock()
	return fmt.Errorf("dynamic TokenSmith auth unhealthy: %w", err)
}

func (m *ServiceTokenManager) errIfUnhealthy() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.unhealthy {
		return nil
	}
	return fmt.Errorf("dynamic TokenSmith auth unhealthy: %s", m.unhealthyErr)
}

func (m *ServiceTokenManager) isUnhealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.unhealthy
}

func (m *ServiceTokenManager) healthStatus() (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.unhealthy, m.unhealthyErr
}

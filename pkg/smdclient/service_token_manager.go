// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openchami/tokensmith/pkg/tokenservice"
)

const (
	defaultServiceIdentity = "metadata-service"
	defaultTargetService   = "smd"
)

// TokenExchangeConfig controls dynamic TokenSmith-backed service-token exchange.
type TokenExchangeConfig struct {
	TokenSmithURL  string
	BootstrapToken string
	TargetService  string
	Scopes         []string

	RefreshBefore           time.Duration
	AutoRefreshInterval     time.Duration
	BootstrapMaxAttempts    int
	BootstrapInitialBackoff time.Duration
	BootstrapMaxBackoff     time.Duration

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
		ServiceName:             defaultServiceIdentity,
		ServiceID:               defaultServiceIdentity,
	}
}

// ServiceTokenManagerStats exposes service-token diagnostics for logging and health checks.
type ServiceTokenManagerStats struct {
	TokenEndpoint string
	TargetService string
	Scopes        []string
	ClientStats   tokenservice.ServiceClientStats
}

// ServiceTokenManager wraps TokenSmith service-token exchange and refresh behavior.
type ServiceTokenManager struct {
	client        *tokenservice.ServiceClient
	tokenEndpoint string
	targetService string
	scopes        []string
}

// NewServiceTokenManager builds a manager around the TokenSmith service client.
func NewServiceTokenManager(config TokenExchangeConfig) *ServiceTokenManager {
	normalized := normalizeTokenExchangeConfig(config)
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

	client := tokenservice.NewServiceClientWithOptions(
		normalized.TokenSmithURL,
		normalized.ServiceName,
		normalized.ServiceID,
		normalized.InstanceID,
		normalized.ClusterID,
		opts...,
	)

	baseURL := strings.TrimRight(strings.TrimSpace(normalized.TokenSmithURL), "/")
	endpoint := baseURL + "/oauth/token"

	return &ServiceTokenManager{
		client:        client,
		tokenEndpoint: endpoint,
		targetService: normalized.TargetService,
		scopes:        append([]string(nil), normalized.Scopes...),
	}
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
	if config.Scopes != nil {
		defaults.Scopes = append([]string(nil), config.Scopes...)
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

	return defaults
}

// Initialize performs the startup bootstrap exchange and retries per config.
func (m *ServiceTokenManager) Initialize(ctx context.Context) error {
	if err := m.client.Initialize(ctx); err != nil {
		return m.wrapEndpointError("bootstrap token exchange", err)
	}
	return nil
}

// GetToken returns a current access token, refreshing when the configured skew is reached.
func (m *ServiceTokenManager) GetToken(ctx context.Context) (string, error) {
	if err := m.client.RefreshTokenIfNeeded(ctx); err != nil {
		return "", m.wrapEndpointError("service token refresh", err)
	}
	token := m.client.GetServiceToken()
	if token == nil || strings.TrimSpace(token.Token) == "" {
		return "", fmt.Errorf("service token unavailable from %s", m.tokenEndpoint)
	}
	return strings.TrimSpace(token.Token), nil
}

// RefreshTokenIfNeeded refreshes the current token if it is near expiry.
func (m *ServiceTokenManager) RefreshTokenIfNeeded(ctx context.Context) error {
	if err := m.client.RefreshTokenIfNeeded(ctx); err != nil {
		return m.wrapEndpointError("service token refresh", err)
	}
	return nil
}

// StartAutoRefresh runs periodic refresh checks until context cancellation.
func (m *ServiceTokenManager) StartAutoRefresh(ctx context.Context) {
	m.client.StartAutoRefresh(ctx)
}

// Stats returns manager-level and TokenSmith client diagnostics.
func (m *ServiceTokenManager) Stats() ServiceTokenManagerStats {
	return ServiceTokenManagerStats{
		TokenEndpoint: m.tokenEndpoint,
		TargetService: m.targetService,
		Scopes:        append([]string(nil), m.scopes...),
		ClientStats:   m.client.Stats(),
	}
}

func (m *ServiceTokenManager) wrapEndpointError(action string, err error) error {
	return fmt.Errorf("%s via %s failed: %w", action, m.tokenEndpoint, err)
}

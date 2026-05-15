// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	defaultSMDSyncIntervalMinutes = 5
	defaultTokenSmithRefreshSkew  = 60
	defaultTokenSmithTarget       = "hsm"
)

type smdRuntime struct {
	client       smdclient.SMDClient
	startWorkers func(context.Context)
}

type tokenSmithConfig struct {
	URL            string
	BootstrapToken string
	TargetService  string
	RefreshSkewSec int
	ScopeHint      string
}

func (c tokenSmithConfig) Enabled() bool {
	return c.URL != "" && c.BootstrapToken != ""
}

type tokenSmithResponse struct {
	Token        string `json:"token"`
	ServiceToken string `json:"service_token"`
	AccessToken  string `json:"access_token"`
	JWT          string `json:"jwt"`
	ExpiresAt    string `json:"expires_at"`
	Expiry       string `json:"expiry"`
	ExpiresIn    int64  `json:"expires_in"`
}

type serviceTokenManager struct {
	cfg    tokenSmithConfig
	client *http.Client

	mu      sync.RWMutex
	token   string
	expires time.Time
}

func newServiceTokenManager(cfg tokenSmithConfig, client *http.Client) *serviceTokenManager {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &serviceTokenManager{
		cfg:    cfg,
		client: client,
	}
}

func (m *serviceTokenManager) Token() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.token
}

func (m *serviceTokenManager) needsRefresh() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.token == "" || m.expires.IsZero() {
		return true
	}
	return time.Until(m.expires) <= time.Duration(m.cfg.RefreshSkewSec)*time.Second
}

func (m *serviceTokenManager) setToken(token string, expiry time.Time) {
	m.mu.Lock()
	m.token = token
	m.expires = expiry
	m.mu.Unlock()
}

func (m *serviceTokenManager) refresh(ctx context.Context) error {
	payload := map[string]string{
		"target_service": m.cfg.TargetService,
	}
	if m.cfg.ScopeHint != "" {
		payload["scope_hint"] = m.cfg.ScopeHint
	}
	if m.cfg.BootstrapToken != "" {
		payload["bootstrap_token"] = m.cfg.BootstrapToken
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal TokenSmith payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create TokenSmith request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if m.cfg.BootstrapToken != "" {
		req.Header.Set("Authorization", "Bearer "+m.cfg.BootstrapToken)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute TokenSmith request: %w", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read TokenSmith response: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close TokenSmith response body: %w", closeErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("TokenSmith exchange failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed tokenSmithResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return fmt.Errorf("decode TokenSmith response: %w", err)
	}

	token := strings.TrimSpace(parsed.Token)
	if token == "" {
		token = strings.TrimSpace(parsed.ServiceToken)
	}
	if token == "" {
		token = strings.TrimSpace(parsed.AccessToken)
	}
	if token == "" {
		token = strings.TrimSpace(parsed.JWT)
	}
	if token == "" {
		return fmt.Errorf("TokenSmith response missing token field")
	}

	expiry := time.Now().Add(time.Hour)
	if parsed.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	} else {
		expiresAt := strings.TrimSpace(parsed.ExpiresAt)
		if expiresAt == "" {
			expiresAt = strings.TrimSpace(parsed.Expiry)
		}
		if expiresAt != "" {
			if parsedTime, err := time.Parse(time.RFC3339, expiresAt); err == nil {
				expiry = parsedTime
			}
		}
	}

	m.setToken(token, expiry)
	return nil
}

func (m *serviceTokenManager) StartRefreshWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !m.needsRefresh() {
					continue
				}
				if err := m.refresh(ctx); err != nil {
					log.Warn().Err(err).Msg("Failed to refresh TokenSmith service token")
				}
			}
		}
	}()
}

func configString(key string, envKeys ...string) string {
	if viper.IsSet(key) {
		value := strings.TrimSpace(viper.GetString(key))
		if value != "" {
			return value
		}
	}
	for _, envKey := range envKeys {
		value := strings.TrimSpace(os.Getenv(envKey))
		if value != "" {
			return value
		}
	}
	return ""
}

func configIntOrDefault(key string, defaultValue int, envKeys ...string) int {
	if viper.IsSet(key) {
		return viper.GetInt(key)
	}
	for _, envKey := range envKeys {
		if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				return parsed
			}
		}
	}
	return defaultValue
}

func configBoolOrDefault(key string, defaultValue bool, envKeys ...string) bool {
	if viper.IsSet(key) {
		return viper.GetBool(key)
	}
	for _, envKey := range envKeys {
		if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" {
			if parsed, err := strconv.ParseBool(raw); err == nil {
				return parsed
			}
		}
	}
	return defaultValue
}

func loadTokenSmithConfig() tokenSmithConfig {
	target := configString("tokensmith_target_service", "TOKENSMITH_TARGET_SERVICE")
	if target == "" {
		target = defaultTokenSmithTarget
	}

	refreshSkew := configIntOrDefault("tokensmith_refresh_skew_sec", defaultTokenSmithRefreshSkew, "TOKENSMITH_REFRESH_SKEW_SEC")
	if refreshSkew < 0 {
		refreshSkew = defaultTokenSmithRefreshSkew
	}

	return tokenSmithConfig{
		URL:            configString("tokensmith_url", "TOKENSMITH_URL"),
		BootstrapToken: configString("tokensmith_bootstrap_token", "TOKENSMITH_BOOTSTRAP_TOKEN"),
		TargetService:  target,
		RefreshSkewSec: refreshSkew,
		ScopeHint:      configString("tokensmith_scope_hint", "TOKENSMITH_SCOPE_HINT"),
	}
}

func smdSyncOptions() smdclient.IntegrationOptions {
	enabled := configBoolOrDefault("smd_sync_enabled", true, "SMD_SYNC_ENABLED")
	intervalMinutes := configIntOrDefault("smd_sync_interval", defaultSMDSyncIntervalMinutes, "SMD_SYNC_INTERVAL")
	if intervalMinutes <= 0 {
		intervalMinutes = defaultSMDSyncIntervalMinutes
	}
	return smdclient.IntegrationOptions{
		SyncEnabled:  enabled,
		SyncInterval: time.Duration(intervalMinutes) * time.Minute,
	}
}

// initSMDRuntime initializes SMD integration based on configuration.
func initSMDRuntime() smdRuntime {
	smdURL := configString("smd_url", "SMD_URL")
	if smdURL == "" {
		log.Warn().Msg("SMD_URL not configured, using mock SMD client for development")
		mock := createMockSMDClient()
		service := smdclient.NewSMDIntegrationService(mock, smdSyncOptions())
		return smdRuntime{
			client: service,
			startWorkers: func(ctx context.Context) {
				service.StartSyncWorker(ctx)
			},
		}
	}

	jwt := configString("smd_jwt", "SMD_JWT")
	if jwt == "" {
		jwt = configString("smd_token", "SMD_TOKEN")
	}

	tokenManager := (*serviceTokenManager)(nil)
	tokenSmith := loadTokenSmithConfig()
	if tokenSmith.Enabled() {
		tokenManager = newServiceTokenManager(tokenSmith, &http.Client{Timeout: 10 * time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := tokenManager.refresh(ctx); err != nil {
			log.Warn().Err(err).Msg("TokenSmith bootstrap exchange failed; starting in degraded mode")
		}
		cancel()
	}

	tokenProvider := func() string {
		if tokenManager != nil {
			if token := strings.TrimSpace(tokenManager.Token()); token != "" {
				return token
			}
		}
		return strings.TrimSpace(jwt)
	}

	log.Info().Msg("SMD_URL configured, using real SMD HTTP client")
	liveClient := smdclient.NewHTTPClientWithTokenProvider(smdURL, tokenProvider)
	service := smdclient.NewSMDIntegrationService(liveClient, smdSyncOptions())

	return smdRuntime{
		client: service,
		startWorkers: func(ctx context.Context) {
			if tokenManager != nil {
				tokenManager.StartRefreshWorker(ctx)
			}
			service.StartSyncWorker(ctx)
		},
	}
}

// createMockSMDClient creates a mock SMD client with sample data for development
func createMockSMDClient() *smdclient.MockSMDClient {
	mock := smdclient.NewMockSMDClient()

	// Add some sample components for testing
	mock.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "b4:2e:99:be:1a:6d",
		IP:   "10.252.0.26",
	})
	mock.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "green"})

	// Add EthernetNICInfo for x1000c0s0b0n0 (2 NICs)
	mock.AddEthernetNICInfo("x1000c0s0b0n0", []smdclient.EthernetNIC{
		{
			RedfishID:           "1",
			Description:         "Node Management Network",
			MACAddress:          "b4:2e:99:be:1a:6d",
			PermanentMACAddress: "b4:2e:99:be:1a:6d",
			InterfaceEnabled:    true,
		},
		{
			RedfishID:           "2",
			Description:         "High Speed Network",
			MACAddress:          "b4:2e:99:be:1a:6e",
			PermanentMACAddress: "b4:2e:99:be:1a:6e",
			InterfaceEnabled:    true,
		},
	})

	// Add EthernetInterfaces for x1000c0s0b0n0 (IP/Network mappings)
	mock.AddEthernetInterfaces("x1000c0s0b0n0", []smdclient.EthernetInterface{
		{
			ID:          "b42e99be1a6d",
			Description: "Node Management Network",
			MACAddress:  "b4:2e:99:be:1a:6d",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.252.0.26", Network: "HMN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a6e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:6e",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.100.0.26", Network: "HSN"},
			},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
	})

	mock.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n1",
		NID:  1001,
		Role: "compute",
		MAC:  "b4:2e:99:be:1a:7d",
		IP:   "10.252.0.27",
	})
	mock.AddGroupMembership("x1000c0s0b0n1", []string{"compute", "blue"})

	// Add EthernetNICInfo for x1000c0s0b0n1 (2 NICs)
	mock.AddEthernetNICInfo("x1000c0s0b0n1", []smdclient.EthernetNIC{
		{
			RedfishID:           "1",
			Description:         "Node Management Network",
			MACAddress:          "b4:2e:99:be:1a:7d",
			PermanentMACAddress: "b4:2e:99:be:1a:7d",
			InterfaceEnabled:    true,
		},
		{
			RedfishID:           "2",
			Description:         "High Speed Network",
			MACAddress:          "b4:2e:99:be:1a:7e",
			PermanentMACAddress: "b4:2e:99:be:1a:7e",
			InterfaceEnabled:    true,
		},
	})

	// Add EthernetInterfaces for x1000c0s0b0n1
	mock.AddEthernetInterfaces("x1000c0s0b0n1", []smdclient.EthernetInterface{
		{
			ID:          "b42e99be1a7d",
			Description: "Node Management Network",
			MACAddress:  "b4:2e:99:be:1a:7d",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.252.0.27", Network: "HMN"},
			},
			ComponentID: "x1000c0s0b0n1",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a7e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:7e",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.100.0.27", Network: "HSN"},
			},
			ComponentID: "x1000c0s0b0n1",
			Type:        "Node",
		},
	})

	mock.AddComponent(&smdclient.Component{
		ID:   "x1000c0s1b0n0",
		NID:  1002,
		Role: "storage",
		MAC:  "b4:2e:99:be:1a:8d",
		IP:   "10.252.0.28",
	})
	mock.AddGroupMembership("x1000c0s1b0n0", []string{"storage"})

	// Add EthernetNICInfo for x1000c0s1b0n0 (1 NIC)
	mock.AddEthernetNICInfo("x1000c0s1b0n0", []smdclient.EthernetNIC{
		{
			RedfishID:           "1",
			Description:         "Node Management Network",
			MACAddress:          "b4:2e:99:be:1a:8d",
			PermanentMACAddress: "b4:2e:99:be:1a:8d",
			InterfaceEnabled:    true,
		},
	})

	// Add EthernetInterfaces for x1000c0s1b0n0
	mock.AddEthernetInterfaces("x1000c0s1b0n0", []smdclient.EthernetInterface{
		{
			ID:          "b42e99be1a8d",
			Description: "Node Management Network",
			MACAddress:  "b4:2e:99:be:1a:8d",
			IPAddresses: []smdclient.IPMapping{
				{IPAddress: "10.252.0.28", Network: "HMN"},
			},
			ComponentID: "x1000c0s1b0n0",
			Type:        "Node",
		},
	})

	log.Info().Msg("Mock SMD client initialized with sample data including EthernetInterface info")
	return mock
}

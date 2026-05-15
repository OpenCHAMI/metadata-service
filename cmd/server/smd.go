// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const defaultSMDSyncIntervalMinutes = 1

type smdRuntime struct {
	client       smdclient.SMDClient
	startWorkers func(context.Context)
}

type smdHealthReporter interface {
	InitialSyncStatus() (bool, string)
}

var currentSMDHealth smdHealthReporter

func initSMDClient(ctx context.Context) (smdclient.SMDClient, error) {
	smdURL := firstConfiguredValue("smd_url", "SMD_URL")
	if smdURL == "" {
		log.Warn().Msg("SMD_URL not configured, using mock SMD client for development")
		return createMockSMDClient(), nil
	}

	client, _, err := initLiveSMDClient(ctx, false)
	return client, err
}

// initSMDRuntime initializes SMD integration and background sync behavior.
func initSMDRuntime() smdRuntime {
	smdURL := firstConfiguredValue("smd_url", "SMD_URL")
	if smdURL == "" {
		log.Warn().Msg("SMD_URL not configured, using mock SMD client for development")
		mock := createMockSMDClient()
		service := smdclient.NewSMDIntegrationService(mock, smdSyncOptions())
		currentSMDHealth = service
		return smdRuntime{
			client: service,
			startWorkers: func(ctx context.Context) {
				service.StartSyncWorker(ctx)
			},
		}
	}

	client, startTokenWorkers, err := initLiveSMDClient(context.Background(), true)
	if err != nil {
		jwt := firstConfiguredValue("smd_jwt", "SMD_JWT")
		if jwt == "" {
			jwt = firstConfiguredValue("smd_token", "SMD_TOKEN")
		}
		log.Warn().Err(err).Msg("Falling back to static SMD HTTP client")
		client = smdclient.NewHTTPClient(smdURL, jwt)
	}

	service := smdclient.NewSMDIntegrationService(client, smdSyncOptions())
	currentSMDHealth = service
	return smdRuntime{
		client: service,
		startWorkers: func(ctx context.Context) {
			if startTokenWorkers != nil {
				startTokenWorkers(ctx)
			}
			service.StartSyncWorker(ctx)
		},
	}
}

func initLiveSMDClient(ctx context.Context, degradeOnTokenSmithFailure bool) (smdclient.SMDClient, func(context.Context), error) {
	smdURL := firstConfiguredValue("smd_url", "SMD_URL")
	jwt := firstConfiguredValue("smd_jwt", "SMD_JWT")
	if jwt == "" {
		jwt = firstConfiguredValue("smd_token", "SMD_TOKEN")
	}

	client := smdclient.NewHTTPClient(smdURL, jwt)
	managerConfig, dynamicEnabled, err := loadTokenExchangeConfig()
	if err != nil {
		if degradeOnTokenSmithFailure {
			log.Warn().Err(err).Msg("TokenSmith configuration invalid; continuing with static SMD auth")
			return client, nil, nil
		}
		return nil, nil, err
	}
	if !dynamicEnabled {
		log.Info().Msg("SMD_URL configured, using real SMD HTTP client with static auth mode")
		return client, nil, nil
	}

	bootstrapCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	manager := smdclient.NewServiceTokenManager(managerConfig)
	if err := manager.Initialize(bootstrapCtx); err != nil {
		if degradeOnTokenSmithFailure {
			log.Warn().Err(err).Msg("TokenSmith bootstrap exchange failed; continuing with static SMD auth")
			return client, nil, nil
		}
		return nil, nil, err
	}

	stats := manager.Stats()
	log.Info().
		Str("tokensmith_url", managerConfig.TokenSmithURL).
		Str("token_endpoint", stats.TokenEndpoint).
		Str("target_service", stats.TargetService).
		Strs("scopes", stats.Scopes).
		Dur("refresh_before", managerConfig.RefreshBefore).
		Msg("SMD_URL configured, using real SMD HTTP client with TokenSmith dynamic auth mode")

	return client.WithServiceTokenManager(manager), func(workerCtx context.Context) {
		manager.StartAutoRefresh(workerCtx)
	}, nil
}

func loadTokenExchangeConfig() (smdclient.TokenExchangeConfig, bool, error) {
	url := firstConfiguredValue("tokensmith_url", "TOKENSMITH_URL")
	if url == "" {
		return smdclient.TokenExchangeConfig{}, false, nil
	}

	bootstrapToken := firstConfiguredValue("tokensmith_bootstrap_token", "TOKENSMITH_BOOTSTRAP_TOKEN")
	if bootstrapToken == "" {
		return smdclient.TokenExchangeConfig{}, true, fmt.Errorf("TokenSmith dynamic auth requires bootstrap token: set tokensmith_bootstrap_token or TOKENSMITH_BOOTSTRAP_TOKEN")
	}

	config := smdclient.DefaultTokenExchangeConfig()
	config.TokenSmithURL = url
	config.BootstrapToken = bootstrapToken
	if targetService := firstConfiguredValue("tokensmith_target_service", "TOKENSMITH_TARGET_SERVICE"); targetService != "" {
		config.TargetService = targetService
	}
	config.Scopes = parseScopes(firstConfiguredValue("tokensmith_scopes", "TOKENSMITH_SCOPES"))

	defaultRefreshBefore := int(config.RefreshBefore / time.Second)
	if skewSeconds := configIntOrDefault("tokensmith_refresh_skew_sec", defaultRefreshBefore, "TOKENSMITH_REFRESH_SKEW_SEC"); skewSeconds > 0 {
		config.RefreshBefore = time.Duration(skewSeconds) * time.Second
	}

	return config, true, nil
}

func firstConfiguredValue(viperKey string, envVars ...string) string {
	if value := strings.TrimSpace(viper.GetString(viperKey)); value != "" {
		return value
	}
	for _, envVar := range envVars {
		if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
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

func parseScopes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	pieces := strings.Split(raw, ",")
	out := make([]string, 0, len(pieces))
	for _, piece := range pieces {
		scope := strings.TrimSpace(piece)
		if scope == "" {
			continue
		}
		out = append(out, scope)
	}
	return out
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
			IPAddresses: []smdclient.IPMapping{{IPAddress: "10.252.0.26", Network: "HMN"}},
			ComponentID: "x1000c0s0b0n0",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a6e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:6e",
			IPAddresses: []smdclient.IPMapping{{IPAddress: "10.100.0.26", Network: "HSN"}},
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
			IPAddresses: []smdclient.IPMapping{{IPAddress: "10.252.0.27", Network: "HMN"}},
			ComponentID: "x1000c0s0b0n1",
			Type:        "Node",
		},
		{
			ID:          "b42e99be1a7e",
			Description: "High Speed Network",
			MACAddress:  "b4:2e:99:be:1a:7e",
			IPAddresses: []smdclient.IPMapping{{IPAddress: "10.100.0.27", Network: "HSN"}},
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
	mock.AddEthernetNICInfo("x1000c0s1b0n0", []smdclient.EthernetNIC{{
		RedfishID:           "1",
		Description:         "Node Management Network",
		MACAddress:          "b4:2e:99:be:1a:8d",
		PermanentMACAddress: "b4:2e:99:be:1a:8d",
		InterfaceEnabled:    true,
	}})

	// Add EthernetInterfaces for x1000c0s1b0n0
	mock.AddEthernetInterfaces("x1000c0s1b0n0", []smdclient.EthernetInterface{{
		ID:          "b42e99be1a8d",
		Description: "Node Management Network",
		MACAddress:  "b4:2e:99:be:1a:8d",
		IPAddresses: []smdclient.IPMapping{{IPAddress: "10.252.0.28", Network: "HMN"}},
		ComponentID: "x1000c0s1b0n0",
		Type:        "Node",
	}})

	log.Info().Msg("Mock SMD client initialized with sample data including EthernetInterface info")
	return mock
}

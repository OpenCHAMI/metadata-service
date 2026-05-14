// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// initSMDClient initializes the SMD client based on configuration
func initSMDClient(ctx context.Context) (smdclient.SMDClient, error) {
	// Check if SMD URL is configured
	smdURL := firstConfiguredValue("smd_url", "SMD_URL")
	if smdURL == "" {
		log.Warn().Msg("SMD_URL not configured, using mock SMD client for development")
		return createMockSMDClient(), nil
	}

	jwt := firstConfiguredValue("smd_jwt", "SMD_JWT")
	if jwt == "" {
		jwt = firstConfiguredValue("smd_token", "SMD_TOKEN")
	}

	tokensmithURL := firstConfiguredValue("tokensmith_url", "TOKENSMITH_URL")
	if tokensmithURL == "" {
		log.Info().Msg("SMD_URL configured, using real SMD HTTP client with static auth mode")
		return smdclient.NewHTTPClient(smdURL, jwt), nil
	}

	bootstrapToken := firstConfiguredValue("tokensmith_bootstrap_token", "TOKENSMITH_BOOTSTRAP_TOKEN")
	if bootstrapToken == "" {
		return nil, fmt.Errorf("TokenSmith dynamic auth requires bootstrap token: set tokensmith_bootstrap_token or TOKENSMITH_BOOTSTRAP_TOKEN")
	}

	managerConfig := smdclient.DefaultTokenExchangeConfig()
	managerConfig.TokenSmithURL = tokensmithURL
	managerConfig.BootstrapToken = bootstrapToken
	managerConfig.TargetService = firstConfiguredValue("tokensmith_target_service", "TOKENSMITH_TARGET_SERVICE")
	if managerConfig.TargetService == "" {
		managerConfig.TargetService = "smd"
	}

	if skewSeconds := viper.GetInt("tokensmith_refresh_skew_sec"); skewSeconds > 0 {
		managerConfig.RefreshBefore = time.Duration(skewSeconds) * time.Second
	}
	managerConfig.Scopes = parseScopes(viper.GetString("tokensmith_scopes"))

	manager := smdclient.NewServiceTokenManager(managerConfig)
	if err := manager.Initialize(ctx); err != nil {
		return nil, err
	}
	go manager.StartAutoRefresh(ctx)

	stats := manager.Stats()
	log.Info().
		Str("tokensmith_url", tokensmithURL).
		Str("token_endpoint", stats.TokenEndpoint).
		Str("target_service", stats.TargetService).
		Strs("scopes", stats.Scopes).
		Dur("refresh_before", managerConfig.RefreshBefore).
		Msg("SMD_URL configured, using real SMD HTTP client with TokenSmith dynamic auth mode")

	client := smdclient.NewHTTPClient(smdURL, jwt).WithServiceTokenManager(manager)
	return client, nil
}

func firstConfiguredValue(viperKey, envVar string) string {
	if value := strings.TrimSpace(viper.GetString(viperKey)); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(envVar))
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

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

	"github.com/OpenCHAMI/cloud-init/pkg/smdclient"
	"github.com/rs/zerolog/log"
)

// initSMDClient initializes the SMD client based on configuration
func initSMDClient(cfg *Config) (smdclient.SMDClient, *smdclient.ServiceTokenManager, error) {
	// Check if SMD URL is configured
	smdURL := os.Getenv("SMD_URL")
	if smdURL == "" {
		log.Warn().Msg("SMD_URL not configured, using mock SMD client for development")
		return createMockSMDClient(), nil, nil
	}

	httpClient := smdclient.NewHTTPClient(smdURL, resolveStaticSMDToken())

	if strings.TrimSpace(cfg.TokenSmithURL) == "" {
		log.Info().Msg("SMD_URL configured, using real SMD HTTP client")
		return httpClient, nil, nil
	}

	bootstrapToken := strings.TrimSpace(cfg.TokenSmithBootstrapToken)
	bootstrapSource := "config"
	if bootstrapToken == "" {
		bootstrapToken = strings.TrimSpace(os.Getenv("TOKENSMITH_BOOTSTRAP_TOKEN"))
		bootstrapSource = "env:TOKENSMITH_BOOTSTRAP_TOKEN"
	}
	if bootstrapToken == "" {
		return nil, nil, fmt.Errorf("tokensmith_url is configured but no bootstrap token was provided")
	}

	tokenCfg := smdclient.DefaultTokenExchangeConfig()
	tokenCfg.TokenSmithURL = strings.TrimSpace(cfg.TokenSmithURL)
	tokenCfg.BootstrapToken = bootstrapToken
	tokenCfg.TargetService = strings.TrimSpace(cfg.TokenSmithTargetService)
	tokenCfg.Scopes = parseScopeCSV(cfg.TokenSmithScopes)
	tokenCfg.RefreshBefore = time.Duration(cfg.TokenSmithRefreshSkewSec) * time.Second

	manager := smdclient.NewServiceTokenManager(tokenCfg)

	log.Info().
		Str("component", "server").
		Str("event", "smd_tokensmith_init").
		Str("endpoint", strings.TrimRight(tokenCfg.TokenSmithURL, "/")+"/service/token").
		Str("target", tokenCfg.TargetService).
		Strs("scopes", tokenCfg.Scopes).
		Bool("bootstrap_token_present", true).
		Str("bootstrap_token_source", bootstrapSource).
		Msg("initializing TokenSmith-backed SMD authentication")

	initialTokenCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := manager.Initialize(initialTokenCtx); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize SMD service token exchange: %w", err)
	}

	httpClient.WithServiceTokenManager(manager)
	log.Info().Msg("SMD_URL configured, using TokenSmith-backed dynamic SMD auth")
	return httpClient, manager, nil
}

func resolveStaticSMDToken() string {
	jwt := strings.TrimSpace(os.Getenv("SMD_JWT"))
	if jwt == "" {
		jwt = strings.TrimSpace(os.Getenv("SMD_TOKEN"))
	}
	return jwt
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

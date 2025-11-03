// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/OpenCHAMI/cloud-init/pkg/smdclient"
	"github.com/rs/zerolog/log"
)

// initSMDClient initializes the SMD client based on configuration
func initSMDClient() smdclient.SMDClient {
	// Check if SMD URL is configured
	smdURL := os.Getenv("SMD_URL")
	if smdURL == "" {
		log.Warn().Msg("SMD_URL not configured, using mock SMD client for development")
		return createMockSMDClient()
	}

	// TODO: Implement real SMD client when needed
	// return smdclient.NewHTTPClient(smdURL)
	log.Warn().Msg("Real SMD client not yet implemented, using mock client")
	return createMockSMDClient()
}

// createMockSMDClient creates a mock SMD client with sample data for development
func createMockSMDClient() *smdclient.MockSMDClient {
	mock := smdclient.NewMockSMDClient()

	// Add some sample components for testing
	mock.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:00",
		IP:   "10.0.0.100",
	})
	mock.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "green"})

	mock.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n1",
		NID:  1001,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:01",
		IP:   "10.0.0.101",
	})
	mock.AddGroupMembership("x1000c0s0b0n1", []string{"compute", "blue"})

	mock.AddComponent(&smdclient.Component{
		ID:   "x1000c0s1b0n0",
		NID:  1002,
		Role: "storage",
		MAC:  "aa:bb:cc:dd:ee:02",
		IP:   "10.0.0.102",
	})
	mock.AddGroupMembership("x1000c0s1b0n0", []string{"storage"})

	log.Info().Msg("Mock SMD client initialized with sample data")
	return mock
}

// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"log"
	"net"

	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

// registerCustomServerIntegrations keeps custom metadata/wireguard wiring out of the generated
// scaffold flow so main.go remains close to Fabrica defaults.
func registerCustomServerIntegrations(r chi.Router) {
	wgController := setupWireGuardController(r)

	if wgController != nil {
		r.Use(wireGuardControllerMiddleware(wgController))
		if viper.GetBool("wireguard_only") {
			r.Use(wireGuardOnlyMiddleware(wgController))
		}
	}

	// Register generated API routes and health endpoint.
	RegisterGeneratedRoutes(r)
	r.Get("/health", healthHandler)

	smdClient := initSMDClient()
	storeAdapter := NewStorageAdapter()

	// Re-register WireGuard routes with SMD client once available.
	if wgController != nil {
		registerWireGuardRoutes(r, wgController, smdClient)
	}

	RegisterCloudInitRoutes(r, smdClient, storeAdapter)
}

func setupWireGuardController(r chi.Router) *wireguard.Controller {
	wgCIDR := viper.GetString("wireguard_server")
	if wgCIDR == "" {
		return nil
	}

	serverIP, network, err := net.ParseCIDR(wgCIDR)
	if err != nil {
		log.Printf("Invalid WIREGUARD_SERVER value: %v", err)
		return nil
	}

	stateFile := viper.GetString("wireguard_state_file")
	controller, err := wireguard.NewController("wg0", serverIP, network, 51820, stateFile)
	if err != nil {
		log.Printf("Failed to initialize WireGuard controller: %v", err)
		return nil
	}

	registerWireGuardRoutes(r, controller, nil)
	log.Printf("WireGuard userspace controller enabled on %s", wgCIDR)
	if stateFile != "" {
		log.Printf("WireGuard state persistence enabled at %s", stateFile)
	}

	return controller
}

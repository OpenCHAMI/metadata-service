// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

func init() {
	rootCmd.Use = "ochami-metadata-server"
	rootCmd.Short = "OpenCHAMI metadata service"
	rootCmd.Long = "ochami-metadata-server - OpenCHAMI metadata service"

	serveCmd.Short = "Start the ochami-metadata-server server"
	serveCmd.Long = "Start the ochami-metadata-server HTTP server with the configured options"
	serveCmd.Flags().String("wireguard-server", "", "Enable WireGuard userspace controller (CIDR, e.g. 100.97.0.1/16)")
}

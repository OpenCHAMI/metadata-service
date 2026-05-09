// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.Use = "ochami-metadata-server"
	rootCmd.Short = "OpenCHAMI metadata service"
	rootCmd.Long = "ochami-metadata-server - OpenCHAMI metadata service"

	serveCmd.Short = "Start the ochami-metadata-server server"
	serveCmd.Long = "Start the ochami-metadata-server HTTP server with the configured options"
	serveCmd.Flags().String("wireguard-server", "", "Enable WireGuard userspace controller (CIDR, e.g. 100.97.0.1/16)")
}

func registerDashAliases(flagSets ...*pflag.FlagSet) {
	for _, flagSet := range flagSets {
		if flagSet == nil {
			continue
		}

		flagSet.VisitAll(func(flag *pflag.Flag) {
			if strings.Contains(flag.Name, "-") {
				viper.RegisterAlias(strings.ReplaceAll(flag.Name, "-", "_"), flag.Name)
			}
		})
	}
}

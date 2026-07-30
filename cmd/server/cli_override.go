// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.Use = "metadata-service"
	rootCmd.Short = "OpenCHAMI metadata service"
	rootCmd.Long = "metadata-service - OpenCHAMI metadata service"

	serveCmd.Short = "Start the metadata-service server"
	serveCmd.Long = "Start the metadata-service HTTP server with the configured options"
	serveCmd.Flags().String("wireguard-server", "", "Enable WireGuard userspace controller (CIDR, e.g. 100.97.0.1/16)")
	serveCmd.Flags().String("tokensmith-url", "", "Enable TokenSmith dynamic SMD authentication via this base URL")
	serveCmd.Flags().String("tokensmith-bootstrap-token", "", "TokenSmith bootstrap token for initial service-token exchange")
	serveCmd.Flags().String("tokensmith-service-identity-cert", "", "Path to TokenSmith mTLS service-identity client certificate PEM")
	serveCmd.Flags().String("tokensmith-service-identity-key", "", "Path to TokenSmith mTLS service-identity client key PEM")
	serveCmd.Flags().String("tokensmith-service-identity-ca", "", "Optional path to TokenSmith CA PEM for mTLS verification override")
	serveCmd.Flags().String("tokensmith-target-service", "smd", "Target downstream service name used for TokenSmith exchange diagnostics")
	serveCmd.Flags().String("tokensmith-scopes", "", "Comma-separated TokenSmith scope metadata for diagnostics")
	serveCmd.Flags().Int("tokensmith-refresh-skew-sec", 300, "Refresh service token when remaining lifetime is below this many seconds")

	if err := bindFlagsWithUnderscoreKeys(viper.GetViper(), serveCmd.Flags()); err != nil {
		panic(fmt.Errorf("failed to bind override flags: %w", err))
	}

	bindServerEnvVars()
}

func bindFlagsWithUnderscoreKeys(v *viper.Viper, flagSets ...*pflag.FlagSet) error {
	var bindErr error

	for _, flagSet := range flagSets {
		if flagSet == nil {
			continue
		}

		flagSet.VisitAll(func(flag *pflag.Flag) {
			if bindErr != nil {
				return
			}

			key := strings.ReplaceAll(flag.Name, "-", "_")
			bindErr = v.BindPFlag(key, flag)
		})
	}

	return bindErr
}

func bindServerEnvVars() {
	mustBindEnv := func(key string, envVars ...string) {
		args := append([]string{key}, envVars...)
		if err := viper.BindEnv(args...); err != nil {
			panic(fmt.Sprintf("failed to bind env for %s: %v", key, err))
		}
	}

	mustBindEnv("smd_url", "SMD_URL")
	mustBindEnv("smd_jwt", "SMD_JWT")
	mustBindEnv("smd_token", "SMD_TOKEN")
	mustBindEnv("tokensmith_url", "TOKENSMITH_URL")
	mustBindEnv("tokensmith_bootstrap_token", "TOKENSMITH_BOOTSTRAP_TOKEN")
	mustBindEnv("tokensmith_service_identity_cert", "TOKENSMITH_SERVICE_IDENTITY_CERT")
	mustBindEnv("tokensmith_service_identity_key", "TOKENSMITH_SERVICE_IDENTITY_KEY")
	mustBindEnv("tokensmith_service_identity_ca", "TOKENSMITH_SERVICE_IDENTITY_CA")
	mustBindEnv("tokensmith_target_service", "TOKENSMITH_TARGET_SERVICE")
	mustBindEnv("tokensmith_scopes", "TOKENSMITH_SCOPES")
	mustBindEnv("tokensmith_refresh_skew_sec", "TOKENSMITH_REFRESH_SKEW_SEC")
}

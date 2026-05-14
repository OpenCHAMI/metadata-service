// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestRegisterDashAliases(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("data-dir", "/data", "")
	flagSet.String("wireguard-state-file", "/data/wireguard/state.yaml", "")
	flagSet.Bool("wireguard-only", false, "")
	flagSet.String("tokensmith-target-service", "smd", "")
	flagSet.Int("tokensmith-refresh-skew-sec", 300, "")

	if err := viper.BindPFlags(flagSet); err != nil {
		t.Fatalf("BindPFlags failed: %v", err)
	}

	registerDashAliases(flagSet)

	viper.Set("data-dir", "/tmp/test-data")
	viper.Set("wireguard-state-file", "/tmp/test-wireguard.yaml")
	viper.Set("wireguard-only", true)
	viper.Set("tokensmith-target-service", "hsm")
	viper.Set("tokensmith-refresh-skew-sec", 42)

	config := DefaultConfig()
	if err := viper.Unmarshal(config); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if config.DataDir != "/tmp/test-data" {
		t.Fatalf("expected DataDir to be /tmp/test-data, got %q", config.DataDir)
	}
	if config.WireGuardStateFile != "/tmp/test-wireguard.yaml" {
		t.Fatalf("expected WireGuardStateFile to be /tmp/test-wireguard.yaml, got %q", config.WireGuardStateFile)
	}
	if !config.WireGuardOnly {
		t.Fatalf("expected WireGuardOnly to be true")
	}
	if config.TokenSmithTargetService != "hsm" {
		t.Fatalf("expected TokenSmithTargetService to be hsm, got %q", config.TokenSmithTargetService)
	}
	if config.TokenSmithRefreshSkewSec != 42 {
		t.Fatalf("expected TokenSmithRefreshSkewSec to be 42, got %d", config.TokenSmithRefreshSkewSec)
	}
}

func TestDefaultConfigUsesAbsoluteDataPaths(t *testing.T) {
	config := DefaultConfig()

	if config.DataDir != "/data" {
		t.Fatalf("expected default DataDir to be /data, got %q", config.DataDir)
	}
	if config.WireGuardStateFile != "/data/wireguard/state.yaml" {
		t.Fatalf("expected default WireGuardStateFile to be /data/wireguard/state.yaml, got %q", config.WireGuardStateFile)
	}
	if config.TokenSmithTargetService != "smd" {
		t.Fatalf("expected default TokenSmithTargetService to be smd, got %q", config.TokenSmithTargetService)
	}
	if config.TokenSmithRefreshSkewSec != 300 {
		t.Fatalf("expected default TokenSmithRefreshSkewSec to be 300, got %d", config.TokenSmithRefreshSkewSec)
	}
}

func TestBindServerEnvVarsForTokenSmith(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("TOKENSMITH_URL", "https://tokensmith.example.com")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "bootstrap-value")
	t.Setenv("TOKENSMITH_TARGET_SERVICE", "smd")
	t.Setenv("TOKENSMITH_SCOPES", "scope:a,scope:b")
	t.Setenv("TOKENSMITH_REFRESH_SKEW_SEC", "75")

	bindServerEnvVars()

	config := DefaultConfig()
	if err := viper.Unmarshal(config); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if config.TokenSmithURL != "https://tokensmith.example.com" {
		t.Fatalf("expected TokenSmithURL env override, got %q", config.TokenSmithURL)
	}
	if config.TokenSmithBootstrapToken != "bootstrap-value" {
		t.Fatalf("expected TokenSmithBootstrapToken env override, got %q", config.TokenSmithBootstrapToken)
	}
	if config.TokenSmithTargetService != "smd" {
		t.Fatalf("expected TokenSmithTargetService env override, got %q", config.TokenSmithTargetService)
	}
	if config.TokenSmithScopes != "scope:a,scope:b" {
		t.Fatalf("expected TokenSmithScopes env override, got %q", config.TokenSmithScopes)
	}
	if config.TokenSmithRefreshSkewSec != 75 {
		t.Fatalf("expected TokenSmithRefreshSkewSec env override, got %d", config.TokenSmithRefreshSkewSec)
	}
}

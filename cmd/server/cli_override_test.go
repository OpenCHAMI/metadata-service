// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestBindFlagsWithUnderscoreKeys_ConfigValuesBeatUnchangedFlagDefaults(t *testing.T) {
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("data-dir", "/data", "")
	flagSet.String("wireguard-state-file", "/data/wireguard/state.yaml", "")
	flagSet.Bool("wireguard-only", false, "")
	flagSet.String("tokensmith-url", "", "")
	flagSet.String("tokensmith-service-identity-cert", "", "")
	flagSet.String("tokensmith-service-identity-key", "", "")
	flagSet.String("tokensmith-service-identity-ca", "", "")
	flagSet.String("tokensmith-target-service", "smd", "")
	flagSet.Int("tokensmith-refresh-skew-sec", 300, "")
	flagSet.Bool("smd-sync-enabled", true, "")
	flagSet.Int("smd-sync-interval", 60, "")

	v := viper.New()
	if err := bindFlagsWithUnderscoreKeys(v, flagSet); err != nil {
		t.Fatalf("bindFlagsWithUnderscoreKeys failed: %v", err)
	}

	v.SetConfigType("yaml")
	configYAML := `
data_dir: /tmp/test-data
wireguard_state_file: /tmp/test-wireguard.yaml
wireguard_only: true
tokensmith_url: https://tokensmith.example.com
tokensmith_service_identity_cert: /certs/client.crt
tokensmith_service_identity_key: /certs/client.key
tokensmith_service_identity_ca: /certs/ca.crt
tokensmith_target_service: hsm
tokensmith_refresh_skew_sec: 42
smd_sync_enabled: false
smd_sync_interval: 30
`
	if err := v.ReadConfig(strings.NewReader(configYAML)); err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}

	config := DefaultConfig()
	if err := v.Unmarshal(config); err != nil {
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
	if config.TokenSmithURL != "https://tokensmith.example.com" {
		t.Fatalf("expected TokenSmithURL to be https://tokensmith.example.com, got %q", config.TokenSmithURL)
	}
	if config.TokenSmithServiceIdentityCert != "/certs/client.crt" {
		t.Fatalf("expected TokenSmithServiceIdentityCert to be /certs/client.crt, got %q", config.TokenSmithServiceIdentityCert)
	}
	if config.TokenSmithServiceIdentityKey != "/certs/client.key" {
		t.Fatalf("expected TokenSmithServiceIdentityKey to be /certs/client.key, got %q", config.TokenSmithServiceIdentityKey)
	}
	if config.TokenSmithServiceIdentityCA != "/certs/ca.crt" {
		t.Fatalf("expected TokenSmithServiceIdentityCA to be /certs/ca.crt, got %q", config.TokenSmithServiceIdentityCA)
	}
	if config.TokenSmithTargetService != "hsm" {
		t.Fatalf("expected TokenSmithTargetService to be hsm, got %q", config.TokenSmithTargetService)
	}
	if config.TokenSmithRefreshSkewSec != 42 {
		t.Fatalf("expected TokenSmithRefreshSkewSec to be 42, got %d", config.TokenSmithRefreshSkewSec)
	}
	if config.SMDSyncEnabled {
		t.Fatal("expected SMDSyncEnabled config value to override unchanged --smd-sync-enabled default")
	}
	if config.SMDSyncInterval != 30 {
		t.Fatalf("expected SMDSyncInterval to be 30, got %d", config.SMDSyncInterval)
	}
}

func TestBindFlagsWithUnderscoreKeys_ChangedHyphenatedFlagsUseUnderscoreKeys(t *testing.T) {
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("data-dir", "/data", "")
	flagSet.String("tokensmith-url", "", "")
	flagSet.Int("smd-sync-interval", 60, "")

	if err := flagSet.Set("data-dir", "/tmp/from-flag"); err != nil {
		t.Fatalf("Set data-dir failed: %v", err)
	}
	if err := flagSet.Set("tokensmith-url", "https://tokensmith.flag"); err != nil {
		t.Fatalf("Set tokensmith-url failed: %v", err)
	}
	if err := flagSet.Set("smd-sync-interval", "15"); err != nil {
		t.Fatalf("Set smd-sync-interval failed: %v", err)
	}

	v := viper.New()
	if err := bindFlagsWithUnderscoreKeys(v, flagSet); err != nil {
		t.Fatalf("bindFlagsWithUnderscoreKeys failed: %v", err)
	}

	if got := v.GetString("data_dir"); got != "/tmp/from-flag" {
		t.Fatalf("expected --data-dir to bind to data_dir, got %q", got)
	}
	if got := v.GetString("tokensmith_url"); got != "https://tokensmith.flag" {
		t.Fatalf("expected --tokensmith-url to bind to tokensmith_url, got %q", got)
	}
	if got := v.GetInt("smd_sync_interval"); got != 15 {
		t.Fatalf("expected --smd-sync-interval to bind to smd_sync_interval, got %d", got)
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
	if config.TokenSmithServiceIdentityCert != "" {
		t.Fatalf("expected default TokenSmithServiceIdentityCert to be empty, got %q", config.TokenSmithServiceIdentityCert)
	}
	if config.TokenSmithServiceIdentityKey != "" {
		t.Fatalf("expected default TokenSmithServiceIdentityKey to be empty, got %q", config.TokenSmithServiceIdentityKey)
	}
	if config.TokenSmithServiceIdentityCA != "" {
		t.Fatalf("expected default TokenSmithServiceIdentityCA to be empty, got %q", config.TokenSmithServiceIdentityCA)
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
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CERT", "/etc/tokensmith/client.crt")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_KEY", "/etc/tokensmith/client.key")
	t.Setenv("TOKENSMITH_SERVICE_IDENTITY_CA", "/etc/tokensmith/ca.crt")
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
	if config.TokenSmithServiceIdentityCert != "/etc/tokensmith/client.crt" {
		t.Fatalf("expected TokenSmithServiceIdentityCert env override, got %q", config.TokenSmithServiceIdentityCert)
	}
	if config.TokenSmithServiceIdentityKey != "/etc/tokensmith/client.key" {
		t.Fatalf("expected TokenSmithServiceIdentityKey env override, got %q", config.TokenSmithServiceIdentityKey)
	}
	if config.TokenSmithServiceIdentityCA != "/etc/tokensmith/ca.crt" {
		t.Fatalf("expected TokenSmithServiceIdentityCA env override, got %q", config.TokenSmithServiceIdentityCA)
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

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
	flagSet.String("tokensmith-bootstrap-policy-scopes-hint", "", "")
	flagSet.String("tokensmith-scopes", "", "")
	flagSet.Int("tokensmith-refresh-skew-sec", 300, "")
	flagSet.Bool("smd-sync-enabled", true, "")
	flagSet.Int("smd-sync-interval", 60, "")
	flagSet.Bool("enable-metrics", false, "")
	flagSet.Int("metrics-port", 9090, "")

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
tokensmith_bootstrap_policy_scopes_hint: metadata:read,groups:read
tokensmith_scopes: legacy:read
tokensmith_refresh_skew_sec: 42
smd_sync_enabled: false
smd_sync_interval: 30
enable_metrics: true
metrics_port: 9191
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
	if config.TokenSmithBootstrapPolicyScopesHint != "metadata:read,groups:read" {
		t.Fatalf("expected TokenSmithBootstrapPolicyScopesHint config value, got %q", config.TokenSmithBootstrapPolicyScopesHint)
	}
	if config.TokenSmithScopesLegacy != "legacy:read" {
		t.Fatalf("expected TokenSmithScopesLegacy config value, got %q", config.TokenSmithScopesLegacy)
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
	if !config.EnableMetrics {
		t.Fatal("expected EnableMetrics config value to override unchanged --enable-metrics default")
	}
	if config.MetricsPort != 9191 {
		t.Fatalf("expected MetricsPort to be 9191, got %d", config.MetricsPort)
	}
}

func TestBindFlagsWithUnderscoreKeys_ChangedHyphenatedFlagsUseUnderscoreKeys(t *testing.T) {
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.String("data-dir", "/data", "")
	flagSet.String("tokensmith-url", "", "")
	flagSet.String("tokensmith-bootstrap-policy-scopes-hint", "", "")
	flagSet.Int("smd-sync-interval", 60, "")
	flagSet.Bool("enable-metrics", false, "")
	flagSet.Int("metrics-port", 9090, "")

	if err := flagSet.Set("data-dir", "/tmp/from-flag"); err != nil {
		t.Fatalf("Set data-dir failed: %v", err)
	}
	if err := flagSet.Set("tokensmith-url", "https://tokensmith.flag"); err != nil {
		t.Fatalf("Set tokensmith-url failed: %v", err)
	}
	if err := flagSet.Set("tokensmith-bootstrap-policy-scopes-hint", "metadata:read,groups:read"); err != nil {
		t.Fatalf("Set tokensmith-bootstrap-policy-scopes-hint failed: %v", err)
	}
	if err := flagSet.Set("smd-sync-interval", "15"); err != nil {
		t.Fatalf("Set smd-sync-interval failed: %v", err)
	}
	if err := flagSet.Set("enable-metrics", "true"); err != nil {
		t.Fatalf("Set enable-metrics failed: %v", err)
	}
	if err := flagSet.Set("metrics-port", "9191"); err != nil {
		t.Fatalf("Set metrics-port failed: %v", err)
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
	if got := v.GetString("tokensmith_bootstrap_policy_scopes_hint"); got != "metadata:read,groups:read" {
		t.Fatalf("expected --tokensmith-bootstrap-policy-scopes-hint to bind to tokensmith_bootstrap_policy_scopes_hint, got %q", got)
	}
	if got := v.GetInt("smd_sync_interval"); got != 15 {
		t.Fatalf("expected --smd-sync-interval to bind to smd_sync_interval, got %d", got)
	}
	if !v.GetBool("enable_metrics") {
		t.Fatal("expected --enable-metrics to bind to enable_metrics")
	}
	if got := v.GetInt("metrics_port"); got != 9191 {
		t.Fatalf("expected --metrics-port to bind to metrics_port, got %d", got)
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
	if config.TokenSmithBootstrapPolicyScopesHint != "" {
		t.Fatalf("expected default TokenSmithBootstrapPolicyScopesHint to be empty, got %q", config.TokenSmithBootstrapPolicyScopesHint)
	}
	if config.TokenSmithScopesLegacy != "" {
		t.Fatalf("expected default TokenSmithScopesLegacy to be empty, got %q", config.TokenSmithScopesLegacy)
	}
	if config.TokenSmithRefreshSkewSec != 300 {
		t.Fatalf("expected default TokenSmithRefreshSkewSec to be 300, got %d", config.TokenSmithRefreshSkewSec)
	}
	if config.EnableMetrics {
		t.Fatal("expected default EnableMetrics to be false")
	}
	if config.MetricsPort != 9090 {
		t.Fatalf("expected default MetricsPort to be 9090, got %d", config.MetricsPort)
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
	t.Setenv("TOKENSMITH_BOOTSTRAP_POLICY_SCOPES_HINT", "scope:a,scope:b")
	t.Setenv("TOKENSMITH_SCOPES", "legacy:a,legacy:b")
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
	if config.TokenSmithBootstrapPolicyScopesHint != "scope:a,scope:b" {
		t.Fatalf("expected TokenSmithBootstrapPolicyScopesHint env override, got %q", config.TokenSmithBootstrapPolicyScopesHint)
	}
	if config.TokenSmithScopesLegacy != "legacy:a,legacy:b" {
		t.Fatalf("expected TokenSmithScopesLegacy env override, got %q", config.TokenSmithScopesLegacy)
	}
	if config.TokenSmithRefreshSkewSec != 75 {
		t.Fatalf("expected TokenSmithRefreshSkewSec env override, got %d", config.TokenSmithRefreshSkewSec)
	}
}

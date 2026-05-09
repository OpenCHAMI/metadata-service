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
	flagSet.String("data-dir", "./data", "")
	flagSet.String("wireguard-state-file", "./data/wireguard/state.yaml", "")
	flagSet.Bool("wireguard-only", false, "")

	if err := viper.BindPFlags(flagSet); err != nil {
		t.Fatalf("BindPFlags failed: %v", err)
	}

	registerDashAliases(flagSet)

	viper.Set("data-dir", "/tmp/test-data")
	viper.Set("wireguard-state-file", "/tmp/test-wireguard.yaml")
	viper.Set("wireguard-only", true)

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
}

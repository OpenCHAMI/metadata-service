// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

type stubSMDHealthReporter struct {
	healthy bool
	reason  string
}

func (s stubSMDHealthReporter) InitialSyncStatus() (bool, string) {
	return s.healthy, s.reason
}

func TestInitConfigLoadsConfigFileAndEnvOverrides(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "server.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 9090\nhost: 127.0.0.1\ndata_dir: /tmp/from-file\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() {
		cfgFile = prevCfgFile
	})

	t.Setenv("METADATA_SERVICE_PORT", "9191")

	initConfig()

	if config == nil {
		t.Fatal("expected config to be initialized")
	}
	if config.Port != 9191 {
		t.Fatalf("expected env override port 9191, got %d", config.Port)
	}
	if config.Host != "127.0.0.1" {
		t.Fatalf("expected host from config file, got %q", config.Host)
	}
	if config.DataDir != "/tmp/from-file" {
		t.Fatalf("expected data_dir from config file, got %q", config.DataDir)
	}
}

func TestInitConfigLoadsConfigFromUserConfigDir(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	configRoot := t.TempDir()
	t.Setenv("HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("AppData", configRoot)

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("resolve user config directory: %v", err)
	}
	configDir := filepath.Join(base, "metadata-service")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create user config directory: %v", err)
	}
	cfgPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 9292\nhost: 127.0.0.2\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = ""
	t.Cleanup(func() {
		cfgFile = prevCfgFile
	})

	t.Chdir(t.TempDir())
	initConfig()

	if config == nil {
		t.Fatal("expected config to be initialized")
	}
	if config.Port != 9292 {
		t.Fatalf("expected port from user config file, got %d", config.Port)
	}
	if config.Host != "127.0.0.2" {
		t.Fatalf("expected host from user config file, got %q", config.Host)
	}
	if used := viper.ConfigFileUsed(); used != cfgPath {
		t.Fatalf("expected user config file %q, got %q", cfgPath, used)
	}
}

func TestHealthHandlerReturnsExpectedPayload(t *testing.T) {
	previous := currentSMDHealth
	currentSMDHealth = nil
	t.Cleanup(func() {
		currentSMDHealth = previous
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}
	var payload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "healthy" {
		t.Fatalf("expected status healthy, got %q", payload["status"])
	}
}

func TestHealthHandlerReturnsUnhealthyBeforeInitialSMDRefresh(t *testing.T) {
	previous := currentSMDHealth
	currentSMDHealth = stubSMDHealthReporter{healthy: false, reason: "smd initial refresh pending"}
	t.Cleanup(func() {
		currentSMDHealth = previous
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "unhealthy" {
		t.Fatalf("expected status unhealthy, got %q", payload["status"])
	}
	if payload["reason"] != "smd initial refresh pending" {
		t.Fatalf("expected reason %q, got %q", "smd initial refresh pending", payload["reason"])
	}
}

func TestDefaultConfigUsesSixtySecondSMDSyncInterval(t *testing.T) {
	if got := DefaultConfig().SMDSyncInterval; got != 60 {
		t.Fatalf("expected default SMD sync interval 60 seconds, got %d", got)
	}
	if got := serveCmd.Flags().Lookup("smd-sync-interval").DefValue; got != "60" {
		t.Fatalf("expected smd-sync-interval flag default 60, got %q", got)
	}
}

func TestRunServerStartsAndShutsDownGracefully(t *testing.T) {
	t.Setenv("SMD_URL", "")
	t.Setenv("SMD_JWT", "")
	t.Setenv("SMD_TOKEN", "")
	t.Setenv("TOKENSMITH_URL", "")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")

	config = &Config{
		Port:         0,
		Host:         "127.0.0.1",
		ReadTimeout:  1,
		WriteTimeout: 1,
		IdleTimeout:  1,
		DataDir:      t.TempDir(),
	}

	origNotify := notifyShutdownSignals
	origStop := stopShutdownSignalNotify
	origRegisterIntegrations := registerServerIntegrations
	defer func() {
		notifyShutdownSignals = origNotify
		stopShutdownSignalNotify = origStop
		registerServerIntegrations = origRegisterIntegrations
	}()

	testSignal := make(chan os.Signal, 1)
	notifyShutdownSignals = func(ch chan<- os.Signal) {
		go func() {
			ch <- <-testSignal
		}()
	}
	stopShutdownSignalNotify = func(chan<- os.Signal) {}
	registerServerIntegrations = func(context.Context, chi.Router) error { return nil }

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(nil, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	testSignal <- syscall.SIGTERM

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runServer returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runServer shutdown")
	}
}

func TestInitSMDRuntimeReturnsFastWithMockSMD(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	originalMockSMD := mockSMD
	mockSMD = true
	t.Cleanup(func() { mockSMD = originalMockSMD })

	t.Setenv("SMD_URL", "")
	t.Setenv("SMD_JWT", "")
	t.Setenv("SMD_TOKEN", "")
	t.Setenv("TOKENSMITH_URL", "")
	t.Setenv("TOKENSMITH_BOOTSTRAP_TOKEN", "")

	// Measure time for initSMDRuntime - should be very fast (< 100ms)
	start := time.Now()
	smdRuntime, err := initSMDRuntime()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("initSMDRuntime returned error: %v", err)
	}
	if smdRuntime.client == nil {
		t.Fatal("expected non-nil client")
	}

	if elapsed > 100*time.Millisecond {
		t.Fatalf("initSMDRuntime took %v, expected < 100ms (non-blocking)", elapsed)
	}

	// Verify startWorkers also returns quickly without blocking
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start = time.Now()
	smdRuntime.startWorkers(ctx)
	elapsed = time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Fatalf("startWorkers took %v, expected < 50ms (non-blocking)", elapsed)
	}
}

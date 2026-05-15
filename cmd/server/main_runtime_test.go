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

	t.Setenv("OCHAMI_METADATA_PORT", "9191")

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

func TestHealthHandlerReturnsExpectedPayload(t *testing.T) {
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

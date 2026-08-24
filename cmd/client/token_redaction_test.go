// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/openchami/metadata-service/pkg/client"
	"github.com/spf13/viper"
)

func TestShowTokenFlagControlsClientDebugLogs(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("show-token")
	if flag == nil {
		t.Fatal("--show-token flag is not registered")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--show-token default = %q, want false", flag.DefValue)
	}

	const token = "cli-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization header = %q, want full token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	originalLogLevel := logLevel
	defer func() { logLevel = originalLogLevel }()
	logLevel = client.LogLevelDebug
	viper.Set("server", server.URL)
	viper.Set("token", token)
	viper.Set("version", "")
	defer func() {
		viper.Set("server", "http://localhost:8080")
		viper.Set("token", "")
		viper.Set("version", "")
		_ = flag.Value.Set("false")
		flag.Changed = false
	}()

	for _, tt := range []struct {
		name string
		show bool
	}{
		{name: "default redaction"},
		{name: "explicit disclosure", show: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := flag.Value.Set(strconv.FormatBool(tt.show)); err != nil {
				t.Fatalf("setting --show-token failed: %v", err)
			}
			flag.Changed = true
			logs := captureStderr(t, func() {
				if err := healthCmd.RunE(healthCmd, nil); err != nil {
					t.Fatalf("health command failed: %v", err)
				}
			})

			if tt.show {
				if !strings.Contains(logs, token) {
					t.Fatalf("full token missing with --show-token: %s", logs)
				}
			} else {
				if strings.Contains(logs, token) {
					t.Fatalf("full token leaked without --show-token: %s", logs)
				}
				if !strings.Contains(logs, "cli-se...") {
					t.Fatalf("redacted token missing from logs: %s", logs)
				}
			}
		})
	}
}

func captureStderr(t *testing.T, run func()) string {
	t.Helper()

	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	os.Stderr = writer
	defer func() { os.Stderr = original }()

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()

	run()
	_ = writer.Close()
	output := <-done
	_ = reader.Close()
	return output
}

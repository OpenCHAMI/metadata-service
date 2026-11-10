// SPDX-FileCopyrightText: © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openchami/fabrica/pkg/versioning"
	_ "github.com/openchami/metadata-service/pkg/apiversion"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openchami/metadata-service/pkg/wireguard"
)

// Config holds all configuration for the service
type Config struct {
	// Server Configuration
	Port         int    `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
	IdleTimeout  int    `mapstructure:"idle_timeout"`

	// Storage, eventing, and metrics settings selected by .fabrica.yaml.
	GeneratedFeatureConfig `mapstructure:",squash"`

	// WireGuard Configuration
	WireGuardStateFile                  string `mapstructure:"wireguard_state_file"`
	WireGuardOnly                       bool   `mapstructure:"wireguard_only"`
	TokenSmithURL                       string `mapstructure:"tokensmith_url"`
	TokenSmithBootstrapToken            string `mapstructure:"tokensmith_bootstrap_token"`
	TokenSmithServiceIdentityCert       string `mapstructure:"tokensmith_service_identity_cert"`
	TokenSmithServiceIdentityKey        string `mapstructure:"tokensmith_service_identity_key"`
	TokenSmithServiceIdentityCA         string `mapstructure:"tokensmith_service_identity_ca"`
	TokenSmithTargetService             string `mapstructure:"tokensmith_target_service"`
	TokenSmithBootstrapPolicyScopesHint string `mapstructure:"tokensmith_bootstrap_policy_scopes_hint"`
	TokenSmithRefreshSkewSec            int    `mapstructure:"tokensmith_refresh_skew_sec"`

	// SMD Integration Configuration
	SMDSyncEnabled  bool `mapstructure:"smd_sync_enabled"`
	SMDSyncInterval int  `mapstructure:"smd_sync_interval"`

	// Feature Flags
	Debug bool `mapstructure:"debug"`

	// EnableMetrics starts the dedicated metrics listener on MetricsPort. The
	// /metrics route on the main listener is selected by .fabrica.yaml at build time.
	EnableMetrics bool `mapstructure:"enable_metrics"`
	MetricsPort   int  `mapstructure:"metrics_port"`
}

// defaultDataDir keeps the service's absolute data path; the generated feature
// default ("./data") suits new services, not packaged deployments.
const defaultDataDir = "/data"

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	config := &Config{
		Port:         8080,
		Host:         "0.0.0.0",
		ReadTimeout:  15,
		WriteTimeout: 15,
		IdleTimeout:  60,

		WireGuardStateFile:                  "/data/wireguard/state.yaml",
		WireGuardOnly:                       false,
		TokenSmithURL:                       "",
		TokenSmithBootstrapToken:            "",
		TokenSmithServiceIdentityCert:       "",
		TokenSmithServiceIdentityKey:        "",
		TokenSmithServiceIdentityCA:         "",
		TokenSmithTargetService:             "smd",
		TokenSmithBootstrapPolicyScopesHint: "",
		TokenSmithRefreshSkewSec:            300,

		SMDSyncEnabled:  true,
		SMDSyncInterval: 60,

		Debug:         false,
		EnableMetrics: false,
		MetricsPort:   9090,
	}
	applyGeneratedFeatureDefaults(config)
	config.DataDir = defaultDataDir
	return config
}

var (
	cfgFile string
	config  *Config
	mockSMD bool
)

var notifyShutdownSignals = func(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
}

var stopShutdownSignalNotify = func(ch chan<- os.Signal) {
	signal.Stop(ch)
}

var registerServerIntegrations = registerCustomServerIntegrations

var currentWGController *wireguard.Controller

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

var rootCmd = &cobra.Command{
	Use:   "github.com/openchami/metadata-service",
	Short: "",
	Long:  `github.com/openchami/metadata-service - A Fabrica-generated OpenCHAMI service`,
	RunE:  runServer,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the github.com/openchami/metadata-service server",
	Long:  `Start the github.com/openchami/metadata-service HTTP server with the configured options`,
	RunE:  runServer,
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /etc/metadata-service/config.yaml)")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&mockSMD, "mock-smd", false, "Use built-in mock SMD data instead of requiring SMD_URL")

	// Server flags
	serveCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	serveCmd.Flags().String("host", "0.0.0.0", "Host to bind to")
	serveCmd.Flags().Int("read-timeout", 15, "Read timeout in seconds")
	serveCmd.Flags().Int("write-timeout", 15, "Write timeout in seconds")
	serveCmd.Flags().Int("idle-timeout", 60, "Idle timeout in seconds")

	// Storage, eventing, and metrics flags selected by .fabrica.yaml
	registerGeneratedFeatureFlags(serveCmd.Flags())
	setFlagDefault(serveCmd.Flags(), "data-dir", defaultDataDir)

	// WireGuard flags
	serveCmd.Flags().String("wireguard-state-file", "/data/wireguard/state.yaml", "Path to WireGuard state file for persistence")
	serveCmd.Flags().Bool("wireguard-only", false, "Restrict access to WireGuard network only")

	// SMD sync flags
	serveCmd.Flags().Bool("smd-sync-enabled", true, "Enable background SMD cache sync worker")
	serveCmd.Flags().Int("smd-sync-interval", 60, "SMD cache sync interval in seconds")

	// Dedicated metrics listener flags
	serveCmd.Flags().Bool("enable-metrics", false, "Start the dedicated metrics listener on --metrics-port")
	serveCmd.Flags().Int("metrics-port", 9090, "Port for the dedicated metrics listener")

	// Bind flags to viper
	if err := bindFlagsWithUnderscoreKeys(viper.GetViper(), serveCmd.Flags(), rootCmd.PersistentFlags()); err != nil {
		panic(fmt.Errorf("failed to bind flags: %w", err))
	}

	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(NewVersionCommand())
	registerGeneratedFeatureCommands(rootCmd)
}

func initConfig() {
	config = DefaultConfig()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/metadata-service")
		if base, err := os.UserConfigDir(); err == nil {
			viper.AddConfigPath(filepath.Join(base, "metadata-service"))
		}
	}

	// Environment variables
	viper.SetEnvPrefix("METADATA_SERVICE")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
	}

	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		log.Fatalf("Unable to decode into config struct: %v", err)
	}

	// Set debug logging
	if config.Debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("Debug logging enabled")
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	log.Printf("Starting github.com/openchami/metadata-service server...")

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Initialize storage, eventing, and metrics selected by .fabrica.yaml.
	features, err := loadGeneratedFeatures(appCtx, config)
	if err != nil {
		return err
	}
	defer features.Close()
	applyEventIdentityDefaults()

	r, err := newRouter(appCtx, config, features)
	if err != nil {
		return err
	}

	if config.EnableMetrics {
		go startMetricsServer(config, newMetricsHandler(features))
	}

	reconcileRuntime, err := newReconciliationRuntimeFn(currentWGController)
	if err != nil {
		return fmt.Errorf("failed to initialize reconciliation runtime: %w", err)
	}
	// Deferred after features.Close so subscriptions are released before the
	// generated cleanup closes the event bus.
	defer func() {
		if stopErr := reconcileRuntime.Stop(); stopErr != nil {
			log.Printf("Failed to stop reconciliation runtime cleanly: %v", stopErr)
		}
	}()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  time.Duration(config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(config.IdleTimeout) * time.Second,
	}

	// Start server in goroutine
	go func() {
		logStartupConfiguration(config, addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	notifyShutdownSignals(quit)
	defer stopShutdownSignalNotify(quit)
	<-quit
	log.Println("Server shutting down...")
	appCancel()

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	log.Println("Server exited")
	return nil
}

// newRouter builds the HTTP route tree. Routes in the protected group sit behind
// the WireGuard middleware (and any generated protected middleware); a route
// belongs in the public group only if callers must reach it unconditionally.
func newRouter(appCtx context.Context, config *Config, features *generatedFeatureSet) (chi.Router, error) {
	r := chi.NewRouter()

	// All middleware must be added before any routes.
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	features.Use(r)
	r.Use(versioning.VersionNegotiationMiddleware(versioning.GlobalVersionRegistry, nil))

	if config.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Group(func(public chi.Router) {
		public.Get("/health", healthHandler)
		RegisterGeneratedPublicRoutes(public)
		public.Get("/openapi.json", ServeOpenAPISpec)
		public.Get("/docs", ServeSwaggerUI)
		features.RegisterPublicRoutes(public)
	})

	var integrationErr error
	r.Group(func(protected chi.Router) {
		features.UseProtected(protected)
		integrationErr = registerServerIntegrations(appCtx, protected)
	})
	if integrationErr != nil {
		return nil, integrationErr
	}
	return r, nil
}

// newMetricsHandler serves the feature-selected public routes, which is where
// the generated /metrics route lives, for the dedicated metrics listener. It
// serves no metrics if .fabrica.yaml disables them.
func newMetricsHandler(features *generatedFeatureSet) http.Handler {
	r := chi.NewRouter()
	features.RegisterPublicRoutes(r)
	return r
}

func startMetricsServer(config *Config, handler http.Handler) {
	metricsAddr := fmt.Sprintf("%s:%d", config.Host, config.MetricsPort)
	log.Printf("Metrics server starting on %s", metricsAddr)

	if err := http.ListenAndServe(metricsAddr, handler); err != nil {
		log.Printf("Metrics server error: %v", err)
	}
}

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	payload := map[string]string{
		"status":  "healthy",
		"service": "github.com/openchami/metadata-service",
	}
	statusCode := http.StatusOK

	if currentSMDHealth != nil {
		if healthy, reason := currentSMDHealth.InitialSyncStatus(); !healthy {
			statusCode = http.StatusServiceUnavailable
			payload["status"] = "unhealthy"
			if reason != "" {
				payload["reason"] = reason
			}
		}
	}

	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

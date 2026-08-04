// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package main provides the cloud-init metadata service server.

This package is the executable entrypoint. It wires server configuration, storage,
middleware, generated REST routes, and custom cloud-init/WireGuard routes.

# Server Architecture

The server is organized into layers:

 1. Configuration (flags, env, config file)
 2. Storage backend initialization
 3. SMD runtime setup (live or mock + optional token exchange)
 4. Middleware chain
 5. Generated REST API routes
 6. Custom cloud-init and optional WireGuard routes

# Configuration

Configuration precedence:
 1. Command-line flags
 2. Environment variables
 3. Config file
 4. Defaults

Common server flags:

	--port (default: 8080)
	--host (default: 0.0.0.0)
	--data-dir (default: /data)
	--read-timeout (default: 15)
	--write-timeout (default: 15)
	--idle-timeout (default: 60)
	--debug (default: false)

SMD and token-exchange inputs are read from explicitly bound keys including:
SMD_URL, SMD_JWT/SMD_TOKEN, TOKENSMITH_URL, TOKENSMITH_BOOTSTRAP_TOKEN,
TOKENSMITH_TARGET_SERVICE, TOKENSMITH_BOOTSTRAP_POLICY_SCOPES_HINT,
and TOKENSMITH_REFRESH_SKEW_SEC.

# Route Structure

The server exposes:

1. Service routes:
  - /health
  - /openapi.json
  - /docs

2. Generated REST resource routes:
  - /clusterdefaultss
  - /groups
  - /instanceinfos
  - /wireguardpeers

3. Custom cloud-init routes:
  - /meta-data
  - /user-data
  - /vendor-data
  - /network-config
  - /{group}.yaml

4. Optional WireGuard routes (when enabled):
  - /wg-init
  - /phone-home/{id}

# Cloud-Init Endpoint Behavior

The /meta-data endpoint:
  - resolves requester identity from forwarded/source IP
  - resolves component identity through SMD
  - builds metadata from ClusterDefaults + InstanceInfo + SMD data
  - returns YAML

Hostname values are synthesized from cluster naming inputs + component NID unless
InstanceInfo explicitly overrides hostname/local_hostname.

The /user-data endpoint returns minimal #cloud-config content.

The /vendor-data endpoint returns #include directives for non-empty group templates.

The /{group}.yaml endpoint:
  - verifies SMD group membership
  - merges runtime metadata context with Group.Spec.MetaData
  - renders template output and returns cloud-config text

# Middleware

Global middleware applied in main:
  - Request logging (chi middleware.Logger)
  - Panic recovery (chi middleware.Recoverer)
  - Request ID (chi middleware.RequestID)
  - RealIP processing (chi middleware.RealIP)

Conditional middleware:
  - /debug profiler mount when --debug is enabled
  - WireGuard context middleware when controller is enabled
  - WireGuard-only access middleware when --wireguard-only is set

# Lifecycle Management

The server supports graceful shutdown on SIGINT/SIGTERM by cancelling app context,
stopping new accepts, and calling http.Server.Shutdown with timeout.

# Storage Initialization

Startup initializes the Fabrica file backend with --data-dir (default /data).
Generated storage helpers persist resources by kind/UID under that backend.

# SMD Runtime Initialization

SMD runtime behavior:
 1. If --mock-smd is set, use the built-in mock SMD client
 2. Otherwise require SMD_URL and initialize a live HTTP SMD client
 3. If TokenSmith settings are valid, enable dynamic service-token mode
 4. Otherwise continue with static SMD auth mode
 5. Wrap with integration service and optionally start sync/token workers

# OpenAPI Documentation

OpenAPI and UI endpoints:

	curl http://localhost:8080/openapi.json
	open http://localhost:8080/docs

# Example Usage

	go run ./cmd/server/main.go serve
	curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8080/meta-data

# Error Handling and Logging

Handlers return standard HTTP statuses and structured JSON/YAML bodies as appropriate.
The service uses standard log plus zerolog in SMD/handler paths.

See README.md and CLOUDINIT.md for operational examples.
*/
package main

<!--
SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.1] - 2026-08-04

### Changed

- Standardized the server environment variable prefix to `METADATA_SERVICE`
- Updated configuration discovery to use `/etc/metadata-service/config.yaml`
  and the platform-specific user configuration directory
- Renamed the built server and client binaries from `ochami-metadata-*` to
  `metadata-service-*`
- Reduced SMD synchronization API calls by using bulk group membership,
  Ethernet interface, and Ethernet NIC lookups while retaining the existing
  cache when bulk synchronization fails
- Updated `github.com/getkin/kin-openapi` from v0.133.0 to v0.144.0
- BREAKING: Changed the Go module and import path from
  `github.com/OpenCHAMI/metadata-service` to
  `github.com/openchami/metadata-service`

### Fixed

- Added support for the standardized non-prefixed TokenSmith environment
  variables, including `TOKENSMITH_BOOTSTRAP_POLICY_SCOPES_HINT`, replacing
  the legacy `TOKENSMITH_SCOPES` variable

## [0.2.0] - 2026-07-22

### Added

- Added WireGuardPeer reconciliation runtime and reconciler support for
  applying persisted WireGuard peer intent to device state
- Added WireGuard peer allocation persistence, deterministic peer UIDs, peer
  status handling, and reconciliation/deletion tests
- Added Prometheus metrics support with `enable_metrics`/`--enable-metrics`,
  `metrics_port`/`--metrics-port`, and `/metrics` endpoints
- Added generated `version` commands and build/version metadata for the server
  and client
- Added generated simple create/update client helpers for resource APIs
- Added architecture, client usage, deployment, FAQ, troubleshooting, and
  WireGuard testing documentation

### Changed

- Regenerated server and client using Fabrica v0.4.9
- Updated cloud-init template handling to return group templates unrendered for
  client-side cloud-init rendering
- Updated template context to use the cloud-init datasource-style `ds` wrapper
- Updated WireGuard initialization to persist `WireGuardPeer` resources and
  return accepted peer allocation responses
- Updated code generation checks to use a locally built Fabrica binary
- Updated Go version to 1.26.5
- Updated `github.com/go-chi/chi/v5` from v5.2.3 to v5.2.4

### Fixed

- Fixed config precedence so underscore-separated YAML and environment keys
  override defaults while CLI flags continue to use hyphen-separated names
- Added `/etc/ochami-metadata` to the server config search path
- Fixed SMD sync startup race by waiting for token readiness before background
  synchronization
- Improved SMD dynamic token logging and diagnostics
- Fixed metadata validation sample data to include expected cloud-init keys
- Fixed WireGuard key handling by converting base64 keys to hex for
  `wireguard-go`
- Fixed WireGuard route registration and middleware ordering issues

## [0.1.2] - 2026-06-17

### Added

- Added YAML struct tags to allow marshalling/unmarshalling YAML data for resources

### Changed

- Regenerated server and client using Fabrica v0.4.8
- Generalized container runtime in Makefile

## [0.1.1] - 2026-06-02

### Added

- Added `--log-level`/`-l` flag and debug messages for client containing HTTP request/response details
- Added client unit tests
- Added SMD integration service with caching and dynamic token support

### Changed

- Regenerated server and client using Fabrica v0.4.7

## [0.1.0] - 2026-05-12

Initial release.

### Added

- Fabrica-generated resource APIs and client commands for `clusterdefaults`, `group`, `instanceinfo`, and `wireguardpeer`.
- NoCloud-style cloud-init endpoints at `/meta-data`, `/user-data`, `/vendor-data`, `/network-config`, and `/{group}.yaml`.
- OpenAPI output at `/openapi.json`, Swagger UI at `/docs`, and a health endpoint at `/health`.
- File-backed persistence under `/data` for generated resources and WireGuard state.
- Mock SMD data for local development when `SMD_URL` is unset.
- Group template validation with Pongo2 rendering, YAML validation, required-variable tracking, and template history/status fields.
- Userspace WireGuard bootstrap support with `/wg-init`, `/phone-home/{id}`, persistence, reverse lookup integration, and generated `wireguardpeer` resources.

### Changed

- Replaced the legacy admin workflow with generated resource APIs and a generated client.
- Standardized server defaults around `/data` storage and explicit dash-to-underscore flag aliases for server configuration.
- Expanded request identity resolution to prefer WireGuard IP reverse lookup before falling back to direct IP lookup.
- Preferred HMN interface data when selecting boot IP and MAC values for metadata generation.

### Fixed

- Filtered `vendor-data` includes so groups without templates stay visible in `/meta-data` but do not produce empty include entries.
- Added fallback IP and MAC resolution from SMD component data when Ethernet interface records are incomplete.
- Corrected local development docs and example scripts to match the current mock SMD addresses, generated client request shape, and WireGuard flag behavior.

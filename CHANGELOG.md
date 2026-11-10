<!--
SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC

SPDX-License-Identifier: MIT
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Before Deploying

- Back up the data directory and plan to roll forward. New resources get
  TypeID UIDs (for example `group_01m738…`) as soon as the upgraded service
  creates them, and no setting disables this. Fabrica does not consider binaries
  without dual-format UID parsing safe rollback targets once TypeIDs are stored.
- **`/metrics` is now always on and public.** It used to be off unless
  `enable_metrics: true` was set. Fabrica's feature loader now enables it at
  build time and serves it on the main port to anyone, with no authentication.
  `wireguard_only` does not cover it: `/metrics` sits with the public routes
  (`/health`, `/service/status`, `/.well-known/api-catalog`, `/openapi.json`,
  `/docs`), which skip the WireGuard check. Sites that need `/metrics` private
  must block it at the network, firewall, or reverse proxy.
- **Metric names changed** from `main_http_*` to `metadata_service_http_*`.
  Update dashboards and alerts, or they will stop matching.
- Clients that create resources without a `spec` must send one; a missing or
  `null` `spec` now returns HTTP 400.
- Treat UIDs as opaque strings.

### Added

- Public `GET /service/status` endpoint backed by SMD readiness
  (`initializing` until the first SMD sync completes, `down` for other SMD or
  TokenSmith failures, `ok` otherwise)
- Public RFC 9727 API catalog at `GET`/`HEAD /.well-known/api-catalog`

### Changed

- Upgraded Fabrica from v0.4.10 to v0.4.15 and Go to 1.26.10
- Adopted Fabrica's generated feature loader: storage, event bus, and metrics
  startup now live in generated `cmd/server/features_generated.go`, so
  `.fabrica.yaml` feature changes no longer require editing `main.go`.
  Commit `cmd/server/features_manifest_generated.json` with generated code
- The WireGuard peer reconciler now subscribes to the generated event bus
  instead of starting its own. Event type prefix (`io.fabrica`), source
  (`metadata-service`), and condition prefix are unchanged; set
  `event_type_prefix` to override the prefix
- Prometheus metrics are selected at build time by `.fabrica.yaml`, use the
  `metadata_service` namespace, and are always served at `/metrics` on the main
  port. `enable_metrics` and `metrics_port` now control only the dedicated
  metrics listener
- Resource creation via the generated API now issues TypeID/UUIDv7 UIDs.
  Existing legacy UIDs are never rewritten and remain fully usable
- `/wg-init` now assigns TypeID UIDs (`wireguardpeer_…`) to new WireGuard
  peers and finds existing peers by public key instead of by a UID derived from
  it. Repeating `/wg-init` with the same key still returns the same peer, and
  existing `wireguardpeer-<hex>` peers keep their UIDs (#72)
- Generated create handlers require the `spec` envelope; conditional
  `If-Match` updates and deletes are atomic, and `If-None-Match` accepts weak
  ETags
- `make generate` now runs the Fabrica version pinned in `go.mod` instead of
  `@latest`

### Fixed

- Fixed the container image build failing with goreleaser 2.20 and later by
  building `linux/amd64` binaries for both the `v1` baseline and `v3`. The
  container image publishes `v1` as `linux/amd64` and `v3` as
  `linux/amd64/v3`, and release archives for `v3` carry a `_v3` suffix
  ([#71](https://github.com/OpenCHAMI/metadata-service/issues/71))
- `go build ./...` and `go vet ./...` failed because the root `doc.go` declared
  `package main` without a `main` function
- `cmd/server/main.go` no longer carries a "Code generated ... DO NOT EDIT"
  header, so linters check it
- `Dockerfile.standalone` defaulted to a Go toolchain older than `go.mod`
  requires
- `/wg-init` created a second peer, with a second VPN IP, for a public key
  already registered through `/wireguardpeers`. Concurrent `/wg-init` calls for
  the same key can no longer create two peers either

### Known Limitations

- `/.well-known/api-catalog` and the OpenAPI `servers` entry report
  `http://localhost:8080` in every deployment (OpenCHAMI/fabrica#170)
- `/wg-init` scans every stored WireGuard peer when it does not find one under
  the legacy UID, so its cost grows with the number of peers
- `/wireguardpeers` does not reject a public key that another peer already
  uses, so a peer created there at the same moment as a `/wg-init` call for
  the same key can still produce a duplicate

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

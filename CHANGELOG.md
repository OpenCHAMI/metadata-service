<!--
SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

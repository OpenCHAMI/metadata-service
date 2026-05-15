// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package cloud-init provides a cloud-init metadata service built on the OpenCHAMI Fabrica framework.

It serves as a modern replacement for the legacy OpenCHAMI/cloud-init service, with generated REST APIs,
resource validation, and integration with the State Management Database (SMD).

# Overview

The service implements a NoCloud-style metadata datasource for HPC node provisioning:
  - /meta-data: Instance metadata (YAML)
  - /user-data: User data (#cloud-config no-op by default)
  - /vendor-data: Include-file directives for group config
  - /network-config: Generated network-config from SMD NIC/interface data
  - /{group}.yaml: Group-specific rendered cloud-config template

When WireGuard userspace mode is enabled, additional endpoints are exposed:
  - /wg-init: Registers a peer and allocates VPN IP
  - /phone-home/{id}: Peer teardown/phone-home flow

# Architecture

The service is organized into key packages:

  - cmd/server: HTTP server bootstrap, route registration, SMD runtime integration
  - pkg/handlers: Cloud-init endpoint handlers
  - apis/cloud-init.openchami.io/v1: Resource schema + validation hooks
  - pkg/resources: Generated registration glue
  - pkg/smdclient: SMD integrations (live and mock)
  - pkg/client: Generated REST client
  - pkg/wireguard: Userspace WireGuard controller

Key technologies:

  - Fabrica framework: Generates REST API handlers/routes/models and storage glue
  - Pongo2 templates: Jinja2-compatible template rendering
  - Chi router: Routing and middleware composition
  - File backend storage: JSON persistence rooted at --data-dir (default /data)

# Resource Model

The service manages four resource types:

  - Group: Template-based group configurations with metadata overlays
  - ClusterDefaults: Cluster-wide defaults injected into metadata/template context
  - InstanceInfo: Per-node overrides (instance IDs, hostnames, base URL, keys)
  - WireGuardPeer: Declarative userspace WireGuard peer records

# Template Variables

Group templates are rendered with merged runtime context including:

	From ClusterDefaults / InstanceInfo:
	- cluster_name
	- base_url
	- cloud_provider
	- region
	- availability_zone
	- public_keys

	From resolved node identity and SMD data:
	- instance_id
	- nid
	- role
	- mac
	- ip
	- interfaces
	- vendor_data (including group metadata)
	- meta_data

	Hostname behavior:
	- hostname and local_hostname are synthesized from cluster naming inputs + component NID
	- if InstanceInfo specifies hostname/local_hostname, those values override synthesis

	Custom variables:
	- user-defined keys from Group.Spec.MetaData

# Node Identification

Request identity is resolved from X-Forwarded-For when present, then request source address.
The service resolves requester IP to component ID through the SMD client resolver path.

# Cloud-Init Integration

The runtime flow is:

1. Resolve requester IP to component ID
2. Load component/group information from SMD
3. Load cluster defaults + optional instance overrides from storage
4. Build metadata/template context
5. Render group templates when requested (/group.yaml)
6. Return cloud-init-compatible responses

Template syntax and rendered YAML validity are validated at Group create/update time.

# Development and Testing

The service only uses the built-in mock SMD client when --mock-smd is set.

Example workflow:

	  # Start server with mock SMD
	  go run ./cmd/server serve --port 8888 --mock-smd

		# Request metadata for a mock node IP
		curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/meta-data

		# Create a group using generated client
		go run ./cmd/client/main.go --server http://localhost:8888 group create \
		  --spec '{"metadata":{"name":"compute"},"spec":{"template":"#cloud-config"}}'

# Code Generation

The Fabrica framework generates several files that should NOT be manually edited:
  - cmd/server/*_handlers_generated.go
  - cmd/server/models_generated.go
  - cmd/server/routes_generated.go
  - internal/storage/storage_generated.go
  - internal/middleware/*_generated.go
  - pkg/client/client_generated.go

To regenerate after modifying resource definitions:

	fabrica generate

# Custom Validation

Resource validation hooks live under apis/cloud-init.openchami.io/v1/*.go.
Notably, Group validation:
1. Extracts referenced template variables
2. Merges sample and group metadata
3. Renders template
4. Validates rendered YAML
5. Tracks template version history

# Storage

Resources are persisted via Fabrica's configured backend (file backend by default under --data-dir).

# Migration from Legacy Service

This service replaces the original OpenCHAMI/cloud-init implementation.

Legacy service:
  - custom/manual routing and handlers
  - manual simulator behavior
  - default port 27777

Current service:
  - generated REST API + custom cloud-init routes
  - resource-level validation hooks
  - explicit mock SMD mode when --mock-smd is set
  - default port 8080 (8888 commonly used for local development)

See LEGACY_COMPATIBILITY.md for compatibility and migration notes.

# Configuration

The service is configured via:
  - command-line flags: go run ./cmd/server serve --help
  - environment variables: OCHAMI_METADATA_* plus explicitly bound keys like SMD_URL,
    SMD_JWT/SMD_TOKEN, and TOKENSMITH_*
  - configuration file: --config

Common options:

	--port: HTTP server port (default: 8080)
	--host: Bind address (default: 0.0.0.0)
	--data-dir: Data storage directory (default: /data)
	--debug: Enable debug logging (default: false)

# Production Considerations

For production deployment:

1. Set SMD_URL to a real SMD instance
2. Configure --data-dir on persistent storage
3. Add external observability/logging
4. Add TLS/access controls at ingress or service layer
5. Review template size/complexity for render performance
6. Validate WireGuard and token-exchange settings if enabled

See README.md and CLOUDINIT.md for endpoint details and deployment usage.
*/
package main

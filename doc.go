// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package cloud-init provides a cloud-init metadata service built on the OpenCHAMI Fabrica framework.

It serves as a modern replacement for the legacy OpenCHAMI/cloud-init service, offering a more robust
and maintainable implementation with automatic REST API generation, comprehensive validation, and
seamless integration with the State Management Database (SMD).

# Overview

The metadata service implements the cloud-init nocloud-net datasource specification, providing
essential cloud-init endpoints for HPC node provisioning:
  - /meta-data: Instance metadata with hostname, instance ID, and cluster information
  - /user-data: User-provided configuration (no-op by default)
  - /vendor-data: Include-file directives pointing to group-specific configurations
  - /{group}.yaml: Group-specific configurations with Jinja2 template support

# Architecture

The service is organized into several key packages:

  - cmd/server: HTTP server implementation with cloud-init endpoints
  - pkg/handlers: Cloud-init endpoint handlers (metadata, user-data, vendor-data)
  - apis/cloud-init.openchami.io/v1: Authoritative resource definitions and validation logic
  - pkg/resources: Generated registration glue for resource wiring
  - pkg/smdclient: Interface for SMD integration with mock implementation for development
  - pkg/client: Generated REST API client for resource management

Key Technologies

  - Fabrica Framework: Automatically generates REST API, storage, and client implementations from
    resource definitions. This eliminates boilerplate and ensures consistency across the service.

  - Pongo2 Templates: Jinja2-compatible template engine for rendering cloud-init configurations.
    Templates support variable substitution from cluster defaults and SMD component data.

  - Chi Router: Lightweight HTTP router for cloud-init endpoint handling with support for
    custom middleware and request transformations.

  - File-Based Storage: JSON-based persistent storage in ./data/{resource-type}/ directory.
    Suitable for development and small deployments; can be extended for other backends.

# Resource Model

The service manages three core resource types:

  - Group: Template-based node group configurations. Each group contains a Jinja2 template that
    produces valid YAML output. Templates are validated on creation/update to ensure they render
    correctly with sample metadata and produce valid YAML.

  - ClusterDefaults: Cluster-wide configuration including base URLs, cloud provider information,
    SSH public keys, and naming conventions. These variables are injected into group templates
    at runtime.

  - InstanceInfo: Per-node instance-specific overrides. Allows customization of hostname, SSH keys,
    and cloud-init URLs on a per-instance basis.

# Template Variables

Group templates have access to the following runtime variables:

	From ClusterDefaults:
	  - cluster_name: Name of the cluster
	  - base_url: Base URL for cloud-init endpoints
	  - cloud_provider: Cloud provider identifier (e.g., "aws", "gcp")
	  - region: Cloud region
	  - availability_zone: Availability zone
	  - short_name: Short cluster name (typically 2 characters)

	From SMD Component:
	  - hostname: Resolved hostname from SMD
	  - instance_id: Unique instance identifier
	  - nid: Node ID in the cluster
	  - role: Node role (e.g., "compute", "login", "storage")

	Custom Variables:
	  - User-defined variables from Group.Spec.MetaData

# Node Identification

Nodes are identified by their IP address, which is resolved through the X-Forwarded-For HTTP header
or request source IP. The SMD client performs IP-to-component lookup to retrieve hardware information,
group membership, and other SMD-managed data.

# Cloud-Init Integration

The service implements a simplified cloud-init datasource that:

1. Receives requests with node IP addresses via X-Forwarded-For header
2. Resolves IP to SMD component ID for hardware lookup
3. Merges cluster defaults, instance-specific data, and group templates
4. Renders group templates with runtime variable injection
5. Validates rendered output as valid YAML
6. Returns properly formatted cloud-init datasource responses

# Development and Testing

For development, the service includes a mock SMD client that provides test nodes without requiring
a running SMD instance. The mock client is automatically enabled when SMD_URL environment variable
is not set.

Example test workflow:

	# Start server with mock SMD
	go run ./cmd/server serve --port 8888

	# Request metadata for mock node
	curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/meta-data

	# Use the generated client for resource management
	go run ./cmd/client/main.go --server http://localhost:8888 group create \
	  --spec '{"name": "compute", "template": "..."}'

# Code Generation

The Fabrica framework generates several files that should NOT be manually edited:
  - cmd/server/*_handlers_generated.go: REST API handlers
  - cmd/server/models_generated.go: Request/response models
  - cmd/server/routes_generated.go: HTTP route definitions
  - internal/storage/storage_generated.go: Storage operations
  - internal/middleware/*_generated.go: Middleware implementations
  - pkg/client/client_generated.go: REST API client

To regenerate after modifying resource definitions:

	fabrica generate

# Custom Validation

Resource validation is customized in apis/cloud-init.openchami.io/v1/*_types.go. The validation process:
1. Parses resource definitions
2. Extracts template variables using regex
3. Renders templates with sample data
4. Validates rendered output as valid YAML
5. Tracks template versions with SHA256 hashing

Example Group validation process:
  - Extracts all {{ variable }} references from template
  - Merges sample cluster defaults and SMD data
  - Attempts template rendering
  - Validates result is valid YAML
  - Updates Status.TemplateHistory with version info

# Storage

Resources are persisted as JSON files in ./data/{resource-type}/:
  - ./data/Group/*.json: Group configurations
  - ./data/ClusterDefaults/*.json: Cluster-wide defaults
  - ./data/InstanceInfo/*.json: Instance-specific overrides

Status fields are automatically updated during validation and persist to storage. The storage
implementation supports file-based persistence with minimal overhead for development.

# Migration from Legacy Service

This service replaces the original OpenCHAMI/cloud-init implementation. Key differences:

Legacy Service:
  - Direct HTTP handlers with custom routing
  - Server-side config merging
  - Group data as JSON with Jinja templates in payloads
  - Base64-encoded content in POST requests
  - Manual SMD simulator mode
  - Runs on port 27777 by default

New Service:
  - Auto-generated REST API from resource definitions
  - Client-side config merging capabilities
  - Resources with built-in validation
  - Plain-text templates validated on creation
  - Mock SMD client for development
  - Runs on port 8080/8888 by default

Most cloud-init endpoints are compatible, but see FABRICA_MIGRATION.md for detailed
compatibility information and migration patterns.

# Configuration

The service is configured via:
  - Command-line flags: go run ./cmd/server serve --help
  - Environment variables: Prefixed with OPENCHAM_CLOUD_INIT_
  - Configuration file: Specified with --config flag

Common configuration options:

	--port: HTTP server port (default: 8080)
	--host: Bind address (default: 0.0.0.0)
	--data-dir: Data storage directory (default: ./data)
	--debug: Enable debug logging (default: false)

For SMD integration:

	SMD_URL: Set to SMD HTTP endpoint (e.g., http://localhost:27779)
	         Unset for mock SMD client in development

# Production Considerations

For production deployment:

1. Set SMD_URL to real SMD instance for hardware lookup
2. Configure --data-dir for persistent storage location
3. Use external logging and monitoring
4. Implement access controls and TLS if needed
5. Template validation runs on every create/update; design templates for performance
6. Group membership authorization enforced via SMD

See README.md for additional deployment and usage information, and CLOUDINIT.md for
detailed cloud-init endpoint specifications.
*/
package main

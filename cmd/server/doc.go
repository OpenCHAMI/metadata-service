// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package main provides the cloud-init metadata service server.

This is the entry point for the cloud-init metadata service. It sets up the HTTP server,
configures all routes (both auto-generated Fabrica routes and custom cloud-init endpoints),
initializes the storage system, and manages the server lifecycle.

# Server Architecture

The server is organized into several layers:

 1. Configuration Layer: Reads config from CLI flags, env vars, and config files
 2. Storage Layer: File-based JSON storage in ./data/ directory
 3. SMD Client Layer: Connection to SMD for hardware component lookup
 4. Middleware Layer: Request logging, validation, versioning
 5. Route Layer: REST API routes (auto-generated) + Cloud-Init routes (custom)
 6. Handler Layer: Business logic for resource operations and cloud-init endpoints

# Configuration

Configuration is managed through multiple sources (in precedence order):
 1. Command-line flags (highest priority)
 2. Environment variables (OPENCHAM_CLOUD_INIT_*)
 3. Configuration file (if provided)
 4. Default values (lowest priority)

Common configuration options:

	--port: HTTP server port (default: 8080)
	--host: Bind address (default: 0.0.0.0)
	--data-dir: Data storage directory (default: ./data)
	--read-timeout: Read timeout in seconds (default: 10)
	--write-timeout: Write timeout in seconds (default: 10)
	--idle-timeout: Idle timeout in seconds (default: 60)
	--debug: Enable debug logging (default: false)

For SMD integration:

	SMD_URL: SMD base URL for production (e.g., http://smd.example.com:27779)
	         Unset for mock SMD in development

# Usage

Start the server:

	go run ./cmd/server/main.go serve

Start with custom port and data directory:

	go run ./cmd/server/main.go serve --port 8888 --data-dir /var/lib/cloud-init

Start with configuration file:

	go run ./cmd/server/main.go serve --config /etc/cloud-init/config.yaml

# Route Structure

The server implements two categories of routes:

1. Auto-Generated Fabrica Routes (REST API for resource management):
  - /groups: Group resource CRUD operations
  - /clusterdefaults: ClusterDefaults resource CRUD operations
  - /instanceinfo: InstanceInfo resource CRUD operations
  - Includes OpenAPI 3.0 schema at /openapi.yaml

2. Custom Cloud-Init Routes (nocloud-net datasource endpoints):
  - /meta-data: Instance metadata
  - /user-data: User-provided configuration
  - /vendor-data: Include-file directives
  - /{group}.yaml: Group-specific template rendering

# Cloud-Init Endpoints

The /meta-data endpoint is the primary cloud-init interface:
  - Returns YAML with instance-id, hostname, cluster information
  - Includes nested instance_data.v1 with vendor_data (group info)
  - Resolves requester IP via X-Forwarded-For header
  - Queries SMD for component information

The /user-data endpoint returns minimal configuration:
  - Returns #cloud-config header only
  - Allows nodes to override with local user-data
  - Compatible with nocloud-net datasource spec

The /vendor-data endpoint includes group configurations:
  - Returns #include directives pointing to /{group}.yaml URLs
  - Enables structured multi-group configuration
  - Compatible with cloud-init include-file processing

The /{group}.yaml endpoint returns rendered group templates:
  - Renders group template with node-specific variables
  - Validates group membership via SMD
  - Returns valid YAML cloud-config
  - Supports caching and conditional requests

# Middleware

The server includes several middleware layers:

  - Logging: All requests logged with zerolog
  - Validation: Request body validation for resource operations
  - CORS: Cross-origin resource sharing support for browser clients
  - Error Handling: Consistent error response formatting
  - Request ID: Unique ID for tracing requests through logs

# Lifecycle Management

The server implements graceful shutdown:
 1. Receives SIGTERM or SIGINT signal
 2. Stops accepting new requests
 3. Waits for in-flight requests to complete (timeout: read/write/idle)
 4. Closes storage connections
 5. Exits cleanly

This ensures no requests are interrupted during deployment updates.

# Storage Initialization

The server initializes file-based storage:
 1. Creates ./data directory if it doesn't exist
 2. Creates subdirectories for each resource type (Group, ClusterDefaults, InstanceInfo)
 3. Loads all existing resources from JSON files
 4. Sets up file watchers for external modifications (optional)

For production, consider backing ./data with persistent volumes or cloud storage.

# SMD Client Initialization

The server initializes the SMD client:
 1. Checks SMD_URL environment variable
 2. If set, creates HTTP client connecting to real SMD
 3. If unset, creates mock client for development with test nodes
 4. Implements retry logic and timeout handling

The mock client provides:
  - Three test nodes: x1000c0s0b0n0, x1000c0s1b0n0, x1000c0s2b0n0
  - IPs: 10.0.0.100, 10.0.0.101, 10.0.0.102
  - Groups: compute, login
  - Realistic component data for testing

# OpenAPI Documentation

The server generates OpenAPI 3.0 schema for all REST API endpoints:

	curl http://localhost:8080/openapi.yaml

Use this schema with API clients and documentation generators:
  - Generate clients in multiple languages: openapi-generator
  - Generate documentation: ReDoc, Swagger UI
  - Validate requests: openapi validator middleware

# Example Usage

Start server with defaults:

	go run ./cmd/server/main.go serve

Query metadata for test node:

	curl -H "X-Forwarded-For: 10.0.0.100" \
	     http://localhost:8080/meta-data

Create a group:

	go run ./cmd/client/main.go --server http://localhost:8080 \
	    group create --spec '{"name":"compute","template":"#cloud-config"}'

Get group list:

	go run ./cmd/client/main.go --server http://localhost:8080 \
	    group list

Get rendered group template:

	curl -H "X-Forwarded-For: 10.0.0.100" \
	     http://localhost:8080/compute.yaml

# Error Handling

The server implements consistent error handling:
  - HTTP status codes follow REST conventions
  - Error responses include descriptive messages
  - Validation errors list specific field issues
  - All errors are logged for debugging

Common error scenarios:
  - 400 Bad Request: Invalid input or missing required fields
  - 404 Not Found: Resource not found or node not in group
  - 409 Conflict: Resource already exists (for create operations)
  - 500 Internal Server Error: Server errors with error details in logs

# Logging

The server uses zerolog for structured logging:
  - All requests logged with method, path, status, duration
  - Errors logged with context and stack traces
  - Custom log levels configurable via CLI flags
  - Structured JSON output suitable for log aggregation

Enable debug logging with --debug flag for verbose output during development.

# Performance Considerations

For production deployment:
 1. Tune timeouts based on network conditions
 2. Configure file system for data directory (SSD recommended)
 3. Consider reverse proxy for TLS termination
 4. Monitor SMD response times and implement caching
 5. Use external log aggregation for audit trails
 6. Implement request rate limiting if needed

The server is designed to handle typical HPC cluster sizes (thousands of nodes)
with reasonable resource consumption. For very large deployments, consider:
  - Database backend instead of file-based storage
  - Distributed architecture with load balancing
  - Template caching strategies
  - SMD client connection pooling

See README.md for deployment guides and examples/ for complete examples.
*/
package main

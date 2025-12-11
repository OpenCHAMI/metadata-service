// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package handlers provides cloud-init datasource endpoint implementations.

This package implements the nocloud-net datasource specification for cloud-init clients,
enabling HPC nodes to receive provisioning and configuration data during boot.

# Endpoints

The package provides handlers for the following cloud-init endpoints:

  - /meta-data: Returns node metadata including instance ID, hostname, cluster information,
    and vendor data with group membership information. This is the primary endpoint used
    by cloud-init to discover node identity and configuration sources.

  - /user-data: Returns user-provided cloud-init configuration. By default, returns an empty
    #cloud-config block, allowing cloud-init clients to override configurations via group
    templates.

  - /vendor-data: Returns include-file directives (#include) that reference group-specific
    configuration URLs. This enables cloud-init clients to fetch group templates from
    the /{group}.yaml endpoints.

  - /{group}.yaml: Returns the rendered group template for the requesting node. Requires
    that the node be a member of the specified group (verified via SMD group membership).

# Request Flow

When a cloud-init client requests metadata:

1. Client sends HTTP request with node IP via X-Forwarded-For header
2. Handler resolves IP to SMD component ID using SMDClient
3. Handler retrieves cluster defaults (global config)
4. Handler retrieves instance-specific config if available
5. Handler merges all configuration sources
6. Handler constructs and returns appropriately formatted response

# Metadata Structure

The /meta-data endpoint returns a MetaData structure with the following information:

  - instance-id: Unique instance identifier
  - local-hostname: Short hostname of the node
  - hostname: Fully qualified domain name
  - cluster-name: Name of the cluster
  - local_ipv4: IP address of the requesting interface
  - instance_data: Nested structure containing vendor data and custom metadata

The instance_data.v1 section includes:
  - vendor_data: Group membership and metadata for cloud-init to process
  - Custom metadata from ClusterDefaults and InstanceInfo

# Template Rendering

For /{group}.yaml endpoints, handlers perform the following:

1. Resolve node IP to SMD component
2. Verify node is member of requested group
3. Retrieve group template definition
4. Merge cluster defaults and SMD component data
5. Render template using Pongo2 (Jinja2-compatible)
6. Return rendered YAML to client

# Group Membership Authorization

Group-specific endpoints enforce authorization through SMD group membership queries.
A node can only access /{group}.yaml if it is listed as a member of that group in SMD.
This enables role-based configuration management (e.g., compute nodes only fetch compute
group configs).

# Error Handling

The handlers implement graceful error handling:

  - 400 Bad Request: Missing or invalid IP address
  - 404 Not Found: Component not found in SMD, group not found, or node not in group
  - 500 Internal Server Error: Template rendering errors, storage errors, SMD connection issues

Handlers log errors using zerolog for debugging and monitoring purposes.

# Template Variables

When rendering group templates, the following variables are available:

	From cluster defaults:
	  - cluster_name: Cluster identifier
	  - base_url: Base URL for cloud-init endpoints
	  - cloud_provider: Cloud provider identifier
	  - region: Cloud region
	  - availability_zone: Availability zone
	  - short_name: Short cluster name (typically 2 chars)
	  - public_keys: List of SSH public keys
	  - nid_length: Expected length of node IDs

	From SMD component:
	  - hostname: Node hostname from SMD
	  - instance_id: Instance identifier
	  - nid: Numeric node ID
	  - role: Node role (compute, login, storage, etc.)

	Custom variables:
	  - From Group.Spec.MetaData defined in group definition

# Design Patterns

The handlers follow these patterns:

  - Node identification: All handlers resolve IP to SMD component for consistent behavior
  - Data merging: Handlers merge cluster defaults, instance info, and group templates
  - Validation: Template rendering validation ensures syntactically valid YAML is returned
  - Logging: All operations are logged for debugging and audit purposes
  - Error recovery: Handlers return appropriate HTTP status codes and error messages

The Store interface provides abstraction for data retrieval, enabling different storage
backends (file-based, database, etc.) without changing handler logic.

See CLOUDINIT.md for detailed endpoint specifications and example requests/responses.
*/
package handlers

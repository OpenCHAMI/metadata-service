<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Cloud-Init Metadata Server Endpoints

This service implements a cloud-init metadata server compatible with the nocloud-net datasource.

## Endpoints

### `/meta-data`

Returns instance metadata in YAML format. The response includes:

- Instance ID
- Hostname information
- Cloud provider details
- Network configuration
- Group memberships

**Authentication**: IP-based (determined from request IP or X-Forwarded-For header)

**Example Response**:

```yaml
instance-id: x1000c0s0b0n0
local-hostname: tc1000
hostname: tc1000
cluster-name: testcluster
instance_data:
  v1:
    cloud-name: OpenCHAMI
    cloud-provider: OpenCHAMI
    region: us-test-1
    availability-zone: test-az-1
    instance-id: x1000c0s0b0n0
    local-hostname: tc1000
    hostname: tc1000
    local-ipv4: 10.0.0.100
    vendor_data:
      version: "1.0"
      cloud_init_base_url: http://cloud-init.local
      cluster_name: testcluster
      nid: 1000
      role: compute
      groups:
        compute:
          description: Compute nodes
          custom_key: custom_value
```

### `/user-data`

Returns user-provided cloud-config data. This endpoint intentionally returns an empty cloud-config to preserve user override capability.

**Response**: `#cloud-config\n`

### `/vendor-data`

Returns an include-file list pointing to group-specific configurations.

**Example Response**:

```yaml
#include
http://cloud-init.local/compute.yaml
http://cloud-init.local/green.yaml
```

### `/{group}.yaml`

Returns group-specific cloud-config with template rendering.

**Path Parameters**:

- `group`: Name of the group

**Authentication**: Verifies node is a member of the requested group

**Template Variables Available**:

- `hostname`: Generated hostname (e.g., `tc1000`)
- `instance_id`: Component ID (e.g., `x1000c0s0b0n0`)
- `nid`: Node ID number
- `role`: Component role
- Plus any custom metadata defined in the group

**Example Response**:

```yaml
#cloud-config
hostname: tc1000
fqdn: tc1000.testcluster.local
```

## Configuration

### Environment Variables

- `SMD_URL`: URL of the State Management Database (SMD) service. If not set, a mock SMD client will be used for development.

### Storage

The service uses three types of resources stored in the configured backend:

#### ClusterDefaults

Cluster-wide configuration including:

- `base_url`: Base URL for cloud-init endpoints
- `cloud_provider`: Cloud provider name
- `region`: Cloud region
- `availability_zone`: Availability zone
- `cluster_name`: Name of the cluster
- `short_name`: Abbreviated cluster name (used in hostname generation)
- `nid_length`: Number of digits for NID padding
- `public_keys`: List of SSH public keys

#### InstanceInfo

Instance-specific configuration (keyed by component ID):

- `instance_id`: Override for instance ID
- `local_hostname`: Override for local hostname
- `hostname`: Override for hostname
- `cloud_init_base_url`: Override for cloud-init base URL
- `public_keys`: Additional SSH public keys

#### Group

Group-specific configuration and templates:

- `description`: Group description
- `template`: Jinja2-compatible template for cloud-config
- `metadata`: Key-value pairs available to templates

## Identity Resolution

The service determines which node is making a request by:

1. Extracting the client IP from the request (supporting X-Forwarded-For for proxied requests)
2. Looking up the component ID in SMD using the IP address
3. Retrieving component information and group memberships

## Testing

Run the comprehensive test suite:

```bash
# All tests
go test ./...

# Handler tests only
go test ./pkg/handlers/ -v

# Group validation tests
go test ./pkg/resources/group/ -v
```

## Development

When `SMD_URL` is not configured, the service uses a mock SMD client with sample data:

- `x1000c0s0b0n0` (10.0.0.100) - compute, green groups
- `x1000c0s0b0n1` (10.0.0.101) - compute, blue groups
- `x1000c0s1b0n0` (10.0.0.102) - storage group

You can test endpoints using curl with the X-Forwarded-For header:

```bash
# Get metadata
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8080/meta-data

# Get vendor-data
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8080/vendor-data

# Get group config
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8080/compute.yaml
```

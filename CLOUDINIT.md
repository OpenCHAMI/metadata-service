<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Cloud-Init Metadata Server Endpoints

This service implements a cloud-init metadata server compatible with the nocloud-net datasource.

## Quick start

1. Start the server with the default mock SMD client (no `SMD_URL` needed):
  ```bash
  go run ./cmd/server serve --port 8888
  ```
2. Call the endpoints using a mock node IP via `X-Forwarded-For`:
  ```bash
  curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/meta-data
  curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/vendor-data
  curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/compute.yaml
  ```
3. Reset state by clearing `./data/` (file-backed storage):
  ```bash
  rm -rf ./data/*
  ```

Mock SMD nodes available out of the box:

| Component ID  | IP           | Groups           |
|---------------|--------------|------------------|
| x1000c0s0b0n0 | 10.0.0.100   | compute, green   |
| x1000c0s0b0n1 | 10.0.0.101   | compute, blue    |
| x1000c0s1b0n0 | 10.0.0.102   | storage          |

## Endpoints

### `/meta-data`

Returns instance metadata in YAML format. The response includes:

- Instance ID
- Hostname information
- Cloud provider details
- Network interface information (from SMD discovery)
- Group memberships

**Authentication**: IP-based (determined from request IP or X-Forwarded-For header)

**Selection policy**: When multiple interfaces are present, the service prefers HMN addresses first, then falls back to the first available IP/MAC.

**Caching**: SMD component and ethernet data is cached for up to 60 seconds to limit SMD request volume.

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
      interfaces:
        - name: eth0
          mac: b4:2e:99:be:1a:6d
          ip: 10.252.0.26
          network: HMN
          description: Node Management Network
          enabled: true
          redfishid: "1"
        - name: eth1
          mac: b4:2e:99:be:1a:6e
          ip: 10.100.0.26
          network: HSN
          description: High Speed Network
          enabled: true
          redfishid: "2"
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

### `/network-config`

Returns cloud-init network configuration (v1 format) with interface details from SMD.

**Authentication**: IP-based (determined from request IP or X-Forwarded-For header)

**Response Format**: cloud-init network-config v1 YAML

**Example Response** (for a 2-NIC node):

```yaml
version: 1
config:
  - type: physical
    name: eth0
    mac_address: b4:2e:99:be:1a:6d
    description: Node Management Network
    subnets:
      - type: static
        address: 10.252.0.26/24
  - type: physical
    name: eth1
    mac_address: b4:2e:99:be:1a:6e
    description: High Speed Network
    subnets:
      - type: static
        address: 10.100.0.26/24
```

**Features**:

- Automatically discovers all network interfaces from SMD's EthernetInterface API
- Maps MAC addresses to IP addresses and networks
- Returns static IP configuration from SMD data
- One endpoint for all network configuration (no templating required)

### `/{group}.yaml`

Returns group-specific cloud-config with template rendering.


**Path Parameters**:

- `group`: Name of the group

**Authentication**: Verifies node is a member of the requested group

**Template encoding**: Set `templateEncoding: base64` on create/update to submit base64-encoded templates. The server decodes and stores plain text.

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
- `SMD_JWT`: JWT to authenticate to SMD (optional).
- `SMD_TOKEN`: Alias for `SMD_JWT` (optional).

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
- `templateEncoding`: Optional encoding for `template` (use `base64` to submit encoded templates)
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

<!--
SPDX-FileCopyrightText: © 2025 OpenCHAMI a Series of LF Projects, LLC

SPDX-License-Identifier: MIT
-->

# Cloud-Init Endpoint Reference

This service implements a NoCloud-style metadata server for cloud-init. It resolves the requesting node through SMD, renders group templates from stored resources, and serves YAML responses suitable for `nocloud-net` bootstrapping.

## Quick Start

1. Start the server with mock SMD enabled explicitly:

   ```bash
  go run ./cmd/server/main.go serve --port 8888 --mock-smd
   ```

2. Query the built-in mock nodes by HMN address through `X-Forwarded-For`:

   ```bash
   curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/meta-data
   curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/user-data
   curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/network-config
   ```

3. Create `ClusterDefaults` and `Group` resources before expecting useful `vendor-data` or `/{group}.yaml` responses. Without stored group resources, `/meta-data` still works, but group metadata and include lists will be empty.

Mock nodes available by default

| Component ID  | HMN IP      | Groups           |
|---------------|-------------|------------------|
| x1000c0s0b0n0 | 10.252.0.26 | compute, green   |
| x1000c0s0b0n1 | 10.252.0.27 | compute, blue    |
| x1000c0s1b0n0 | 10.252.0.28 | storage          |

## Endpoint Behavior

### `/meta-data`

Returns cloud-init metadata in YAML format.

Current behavior
- Resolves the node by request IP, honoring `X-Forwarded-For`
- Prefers a stored WireGuard IP to component lookup before falling back to direct IP lookup
- Prefers HMN data when choosing the boot IP and MAC for a component with multiple interfaces
- Includes every group membership in `instance_data.v1.vendor_data.groups`, even if a group has no template

Example response

```yaml
instance-id: x1000c0s0b0n0
local-hostname: tc1000
hostname: tc1000
cluster-name: testcluster
instance_data:
  v1:
    cloud-name: OpenCHAMI
    cloud-provider: OpenCHAMI
    region: lab
    availability-zone: lab-a
    instance-id: x1000c0s0b0n0
    local-hostname: tc1000
    hostname: tc1000
    local-ipv4: 10.252.0.26
    public_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyExample demo@example
    vendor_data:
      version: "1.0"
      cloud_init_base_url: http://localhost:8888
      cluster_name: testcluster
      nid: 1000
      role: compute
      mac: b4:2e:99:be:1a:6d
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
          scheduler: slurm
        green:
          description: Green nodes
          color: green
```

### `/user-data`

Returns a fixed empty cloud-config:

```yaml
#cloud-config
```

This endpoint is intentionally a no-op so user overrides can still be layered by the client.

### `/vendor-data`

Returns a `#include` list of group templates for the requesting node.

Current behavior
- Uses `ClusterDefaults.Spec.BaseURL` or `InstanceInfo.Spec.CloudInitBaseURL` when building URLs
- Includes only groups with non-empty templates
- Still exposes metadata for empty-template groups in `/meta-data`

Example response after creating `compute` and `green` groups:

```yaml
#include
http://localhost:8888/compute.yaml
http://localhost:8888/green.yaml
```

### `/network-config`

Returns cloud-init network-config v1 YAML derived from SMD Ethernet data.

Current behavior
- Emits one `physical` config item per discovered interface
- Uses the interface MAC, description, and IP from SMD
- Emits static subnet entries using the discovered address with a `/24` mask
- Returns `version: 1` with an empty `config` list when no interface data is available

Example response:

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

### `/{group}.yaml`

Returns the rendered template for a group the node belongs to.

Current behavior
- Rejects requests for groups the node is not a member of
- Renders the stored plain-text template with merged runtime metadata
- Uses the same identity resolution path as `/meta-data`

## Template Variables

Group templates access runtime data through the `ds` (datasource) namespace, following cloud-init conventions. All template variables are prefixed with `ds.` to access the datasource context.

### Available Template Variables

Templates use Pongo2 (Jinja2-compatible) syntax. All variables are accessed via the `ds` prefix:

**Identity & Networking:**
- `{{ ds.hostname }}` - Generated hostname (e.g., `tc1000`)
- `{{ ds.local_hostname }}` - Local hostname alias
- `{{ ds.instance_id }}` - Component XName (e.g., `x1000c0s0b0n0`)
- `{{ ds.nid }}` - Numeric node ID (e.g., `1000`)
- `{{ ds.role }}` - Node role from SMD (e.g., `compute`)
- `{{ ds.mac }}` - Primary MAC address
- `{{ ds.ip }}` - Primary IP address
- `{{ ds.interfaces }}` - List of network interfaces with MAC/IP/network

**Cluster Configuration:**
- `{{ ds.cluster_name }}` - Full cluster name (e.g., `production`)
- `{{ ds.base_url }}` - Cloud-init server URL
- `{{ ds.cloud_provider }}` - Provider name (typically `OpenCHAMI`)
- `{{ ds.region }}` - Cluster region
- `{{ ds.availability_zone }}` - Availability zone

**SSH Keys:**
Arrays require a for loop in templates:
```yaml
users:
  - name: root
    ssh_authorized_keys:{% for key in ds.meta_data.instance_data.v1.public_keys %}
      - {{ key }}{% endfor %}
```

**Full Metadata Access:**
- `{{ ds.meta_data }}` - Complete metadata document as nested YAML
- `{{ ds.vendor_data }}` - Vendor-specific data including group metadata

**Custom Variables:**
Access custom keys from `Group.Spec.MetaData` directly by name:
```yaml
# If Group.Spec.MetaData = {"scheduler": "slurm"}
scheduler: {{ scheduler }}
```

### Example Template

```yaml
#cloud-config
hostname: {{ ds.hostname }}
fqdn: {{ ds.hostname }}.{{ ds.cluster_name }}.local

users:
  - name: root
    ssh_authorized_keys:{% for key in ds.meta_data.instance_data.v1.public_keys %}
      - {{ key }}{% endfor %}

write_files:
  - path: /etc/node-info
    content: |
      NODE_ID={{ ds.instance_id }}
      NODE_NID={{ ds.nid }}
      NODE_ROLE={{ ds.role }}
      CLUSTER={{ ds.cluster_name }}
      PRIMARY_IP={{ ds.ip }}
      PRIMARY_MAC={{ ds.mac }}
```

### Example Response

```yaml
#cloud-config
hostname: tc1000
fqdn: tc1000.production.local

users:
  - name: root
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC... admin@production
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... deploy@automation

write_files:
  - path: /etc/node-info
    content: |
      NODE_ID=x1000c0s0b0n0
      NODE_NID=1000
      NODE_ROLE=compute
      CLUSTER=production
      PRIMARY_IP=10.252.0.26
      PRIMARY_MAC=b4:2e:99:be:1a:6d
```

## Resource Inputs Used By The Endpoints

### ClusterDefaults

Relevant fields from `ClusterDefaults.Spec`
- `base_url`
- `cloud_provider`
- `region`
- `availability_zone`
- `cluster_name`
- `short_name`
- `nid_length`
- `public_keys`

### InstanceInfo

Relevant fields from `InstanceInfo.Spec`
- `instance_id`
- `local_hostname`
- `hostname`
- `cloud_init_base_url`
- `public_keys`

### Group

Relevant fields from `Group.Spec`
- `description`
- `template`
- `metaData`
- `osVersion`

Group templates are stored and rendered as plain text. There is no `templateEncoding` field in the current API.

## Identity Resolution

For both cloud-init and network-config requests, the server determines the node by:
1. Reading `X-Forwarded-For` if present, otherwise the request remote address.
2. Looking up a component by WireGuard-assigned IP if one exists.
3. Falling back to a direct SMD IP lookup.
4. Querying SMD for component data, group membership, and Ethernet interfaces.

## Related Service Endpoints

- `/health`
- `/openapi.json`
- `/docs`

## Validation And Testing

Run the project test suite with:

```bash
make test
```

Useful focused checks:

```bash
go test ./pkg/handlers/... -v
go test ./cmd/server/... -v
```

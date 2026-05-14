<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Cloud-Init Endpoint Reference

This service implements a NoCloud-style metadata server for cloud-init. It resolves the requesting node through SMD, renders group templates from stored resources, and serves YAML responses suitable for `nocloud-net` bootstrapping.

## Quick Start

1. Start the server with mock SMD enabled automatically:

   ```bash
   go run ./cmd/server/main.go serve --port 8888
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

Template context available to group templates
- Flat keys: `hostname`, `local_hostname`, `instance_id`, `cluster_name`, `cloud_name`, `cloud_provider`, `availability_zone`, `region`, `local_ipv4`, `base_url`, `nid`, `role`, `mac`, `ip`, `interfaces`, and `public_keys`
- Nested `vendor_data` matching the vendor-data section of `/meta-data`
- Nested `meta_data` containing the full cloud-init metadata document
- Custom keys from `Group.Spec.MetaData`

Example response:

```yaml
#cloud-config
hostname: tc1000
write_files:
  - path: /etc/node-role
    content: |
      ROLE=compute
      NID=1000
      IP=10.252.0.26
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

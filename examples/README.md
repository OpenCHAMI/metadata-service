<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Example Scripts

This directory contains two shell scripts for exercising the metadata service against the built-in mock SMD dataset.

## Mock Nodes

The mock SMD client created by the server exposes these nodes by default:

| Component ID  | HMN IP      | Groups           |
| --- | --- | --- |
| x1000c0s0b0n0 | 10.252.0.26 | compute, green   |
| x1000c0s0b0n1 | 10.252.0.27 | compute, blue    |
| x1000c0s1b0n0 | 10.252.0.28 | storage          |

## Scripts

### `quick-test.sh`

Runs a lightweight health check against a running server and prints:
- `/health`
- `/meta-data`
- `/user-data`
- `/network-config`

This script does not create any resources. It is useful for verifying that mock SMD identity resolution and the built-in handlers are working.

### `demo.sh`

Creates a small end-to-end demo environment by:
- creating one `ClusterDefaults` resource
- creating `compute`, `green`, `blue`, and `storage` group resources that match the built-in mock memberships
- exercising `/meta-data`, `/vendor-data`, `/network-config`, and several rendered `/{group}.yaml` endpoints
- creating an `InstanceInfo` override for `x1000c0s0b0n0`

The demo uses the generated client with the current request shape, which means every create operation sends both `metadata` and `spec`.

## Running The Scripts

Start the server from the repository root:

```bash
go run ./cmd/server/main.go serve --port 8888
```

Run the quick verification:

```bash
cd examples
./quick-test.sh
```

Run the end-to-end demo:

```bash
cd examples
./demo.sh
```

## Manual Checks

Before creating resources:

```bash
curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/meta-data
curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/network-config
```

After running `demo.sh`:

```bash
curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/vendor-data
curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/compute.yaml
curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/green.yaml
curl -H "X-Forwarded-For: 10.252.0.27" http://localhost:8888/blue.yaml
curl -H "X-Forwarded-For: 10.252.0.28" http://localhost:8888/storage.yaml
```

## Cleanup

The default storage directory is `/data`. To reset local state:

```bash
rm -rf /data/*
```

If you started the server on a different port, update `SERVER_URL` in the scripts or export it before running them.

# Update the group
go run ../cmd/client/main.go --server http://localhost:8888 group update compute-nodes --spec "$(cat current.json)"
```

## Real-World Usage

In production, you would:

1. **Configure Real SMD**: Set `SMD_URL` environment variable to point to your State Management Database
2. **Create Groups**: Define groups matching your cluster organization
3. **Assign Nodes**: Use SMD to assign nodes to appropriate groups
4. **Deploy Configs**: Nodes fetch their configuration via cloud-init on boot
5. **Update Templates**: Modify group templates and nodes pick up changes on next boot

## See Also

- [Main README](../README.md) - Project overview
- [CLOUDINIT.md](../CLOUDINIT.md) - Cloud-init endpoint documentation
- [Generated Client](../cmd/client/) - CLI client source code
- [API Server](../cmd/server/) - Server source code

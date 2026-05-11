<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# github.com/OpenCHAMI/metadata-service

Cloud-init metadata service built on the OpenCHAMI Fabrica framework. It serves nocloud-net metadata endpoints for HPC nodes, renders Jinja2/Pongo2 templates for group configs, and integrates with a State Management Database (SMD) or a mock client for local development. It is a drop-in replacement for the legacy `cloud-init/admin` service with stronger validation and generated APIs.

Key capabilities
- Auto-generated REST resources for groups, cluster defaults, and instance info
- Server-side template validation (Jinja2-compatible, YAML-safe)
- SMD-backed identity and group membership with mock mode when `SMD_URL` is unset
- File-based storage with persistence under `/data/`
- Optional userspace WireGuard controller for compatible VPN-style bootstrapping



## Quick start (mock SMD)

1. Ensure Go 1.22+ is installed.
2. From the repo root, start the server (mock SMD is automatic when `SMD_URL` is unset):
	```bash
	go run ./cmd/server serve --port 8888
	```
3. Hit the cloud-init endpoints using the bundled mock nodes:
	```bash
	curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/meta-data
	curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/vendor-data
	curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/compute.yaml
	```
4. Data lives under `/data/`. Remove it to reset state:
	```bash
	rm -rf /data/*
	```

Mock nodes available by default:
- `10.0.0.100` (`x1000c0s0b0n0`): groups compute, green
- `10.0.0.101` (`x1000c0s0b0n1`): groups compute, blue
- `10.0.0.102` (`x1000c0s1b0n0`): group storage

## Manage resources with the generated client

Use the generated CLI to create or update cluster defaults, groups, and instance overrides.

```bash
# Create cluster defaults
go run ./cmd/client --server http://localhost:8888 clusterdefaults create --spec '{"name":"demo","base_url":"http://localhost:8888","cloud_provider":"OpenCHAMI","region":"dev","nid_length":4,"public_keys":["ssh-ed25519 AAA... user@example"]}'

# Create a group with a simple template
cat > /tmp/compute.json <<'EOF'
{
  "name": "compute",
  "description": "Compute nodes",
  "template": "#cloud-config\nhostname: {{ hostname }}\nusers:\n  - name: hpc\n    ssh-authorized-keys: {{ metadata.public_keys }}\n",
  "metadata": {
	 "public_keys": ["ssh-ed25519 AAA... user@example"]
  }
}
EOF
go run ./cmd/client --server http://localhost:8888 group create --spec "$(cat /tmp/compute.json)"

# Create a group with base64-encoded template content
cat > /tmp/compute-b64.json <<'EOF'
{
	"name": "compute",
	"description": "Compute nodes",
	"template": "I2Nsb3VkLWNvbmZpZ1xuaG9zdG5hbWU6IHt7IGhvc3RuYW1lIH19XG4=",
	"templateEncoding": "base64",
	"metadata": {
		"public_keys": ["ssh-ed25519 AAA... user@example"]
	}
}
EOF
go run ./cmd/client --server http://localhost:8888 group create --spec "$(cat /tmp/compute-b64.json)"

# Optional: instance-specific overrides
go run ./cmd/client --server http://localhost:8888 instanceinfo create --spec '{"name":"x1000c0s0b0n0","hostname":"custom-host"}'
```

## Cloud-init endpoints

The service implements nocloud-net compatible endpoints. See [CLOUDINIT.md](CLOUDINIT.md) for full details and response examples.

- `/meta-data` — YAML metadata for the requesting node (IP- or X-Forwarded-For-based identity)
- `/vendor-data` — include file list of group YAMLs
- `/user-data` — empty `#cloud-config` to preserve user overrides
- `/{group}.yaml` — rendered group template (requires group membership)

## Running with real SMD

Set `SMD_URL` to point at your SMD service. Identity, group membership, and overrides will be sourced from SMD instead of the mock client.
If your SMD requires authentication, provide a JWT via `SMD_JWT` (or `SMD_TOKEN`).

```bash
SMD_URL=https://smd.example.com SMD_JWT="$JWT" go run ./cmd/server serve --port 8888
```

## Optional: userspace WireGuard endpoints

Enable with `--wireguard_server` to expose `/wg-init` and `/phone-home/{id}` for userspace WireGuard bootstrapping.

```bash
go run ./cmd/server serve --port 8888 --wireguard_server
```

## Development

```bash
# Install dependencies
go mod tidy

# Regenerate Fabrica code (default: released module)
make generate

# Run the server
go run ./cmd/server serve --port 8888

# Run tests (skips integration tests that need a legacy server)
make test
```

### Using GoReleaser
OpenCHAMI employs [GoReleaser](https://goreleaser.com/) for automated releases and build metadata tracking.

To build locally:
#### Set Environment Variables
```bash
export GIT_STATE=$(if git diff-index --quiet HEAD --; then echo 'clean'; else echo 'dirty'; fi)
export BUILD_HOST=$(hostname)
export GO_VERSION=$(go version | awk '{print $3}')
export BUILD_USER=$(whoami)
```

#### Install GoReleaser
Follow [GoReleaser’s installation guide](https://goreleaser.com/install/).

#### Build Locally
```bash
goreleaser release --snapshot --clean
```
Built binaries will be located in the `dist/` directory.

## Legacy parity highlights

- Same nocloud-net endpoints as the legacy service (`/meta-data`, `/user-data`, `/vendor-data`, `/{group}.yaml`)
- Server-side template validation (Jinja2/Pongo2 + YAML) prevents invalid configs at create time
- Mock SMD replaces legacy impersonation routes; use `X-Forwarded-For` to simulate callers
- Resources created via generated client (`clusterdefaults`, `group`, `instanceinfo`) instead of legacy `/cloud-init/admin/*` routes

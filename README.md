<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# OpenCHAMI Metadata Service

The OpenCHAMI metadata service provides NoCloud-compatible cloud-init endpoints for HPC nodes and a generated resource API for the data those endpoints render. It is built on Fabrica, stores resources on disk, integrates with SMD for node identity and group membership, and falls back to a mock SMD client for local development when `SMD_URL` is unset.

Key capabilities
- NoCloud-style endpoints: `/meta-data`, `/user-data`, `/vendor-data`, `/network-config`, and `/{group}.yaml`
- Generated resource APIs and client commands for `clusterdefaults`, `group`, `instanceinfo`, and `wireguardpeer`
- Server-side template validation for group cloud-config templates using Pongo2 plus YAML validation
- OpenAPI output at `/openapi.json` and Swagger UI at `/docs`
- Optional userspace WireGuard bootstrap endpoints at `/wg-init` and `/phone-home/{id}`

## Quick Start

The server defaults to port `8080`. The examples below use `8888` explicitly.

1. Start the server with the built-in mock SMD data:

	 ```bash
	 go run ./cmd/server/main.go serve --port 8888
	 ```

2. Verify the service endpoints that work without any stored resources:

	 ```bash
	 curl http://localhost:8888/health
	 curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/meta-data
	 curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/user-data
	 curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/network-config
	 ```

3. Create the minimum resources needed for meaningful `vendor-data` and group template rendering. The generated client expects a full request object with both `metadata` and `spec`.

	 ```bash
	 cat > /tmp/clusterdefaults.json <<'EOF'
	 {
		 "metadata": {
			 "name": "demo-cluster"
		 },
		 "spec": {
			 "description": "Local demo cluster defaults",
			 "base_url": "http://localhost:8888",
			 "cloud_provider": "OpenCHAMI",
			 "region": "lab",
			 "availability_zone": "lab-a",
			 "cluster_name": "testcluster",
			 "short_name": "tc",
			 "nid_length": 4,
			 "public_keys": [
				 "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyExample demo@example"
			 ]
		 }
	 }
	 EOF
	 go run ./cmd/client/main.go --server http://localhost:8888 clusterdefaults create --spec "$(cat /tmp/clusterdefaults.json)"

	 cat > /tmp/compute-group.json <<'EOF'
	 {
		 "metadata": {
			 "name": "compute"
		 },
		 "spec": {
			 "description": "Compute nodes",
			 "template": "#cloud-config\nhostname: {{ hostname }}\nwrite_files:\n  - path: /etc/node-role\n    content: |\n      ROLE={{ role }}\n      NID={{ nid }}\n      IP={{ ip }}\n",
			 "metaData": {
				 "scheduler": "slurm"
			 }
		 }
	 }
	 EOF
	 go run ./cmd/client/main.go --server http://localhost:8888 group create --spec "$(cat /tmp/compute-group.json)"

	 cat > /tmp/green-group.json <<'EOF'
	 {
		 "metadata": {
			 "name": "green"
		 },
		 "spec": {
			 "description": "Green nodes",
			 "template": "#cloud-config\nwrite_files:\n  - path: /etc/node-color\n    content: |\n      COLOR={{ color }}\n",
			 "metaData": {
				 "color": "green"
			 }
		 }
	 }
	 EOF
	 go run ./cmd/client/main.go --server http://localhost:8888 group create --spec "$(cat /tmp/green-group.json)"
	 ```

4. With those resources in place, test the rendered cloud-init flows for the first mock node:

	 ```bash
	 curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/vendor-data
	 curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/compute.yaml
	 curl -H "X-Forwarded-For: 10.252.0.26" http://localhost:8888/green.yaml
	 ```

Mock SMD nodes available by default
- `x1000c0s0b0n0` at `10.252.0.26` with groups `compute`, `green`
- `x1000c0s0b0n1` at `10.252.0.27` with groups `compute`, `blue`
- `x1000c0s1b0n0` at `10.252.0.28` with group `storage`

## API Surface

Public service endpoints
- `/health`
- `/openapi.json`
- `/docs`

Cloud-init endpoints
- `/meta-data`
- `/user-data`
- `/vendor-data`
- `/network-config`
- `/{group}.yaml`

Generated resource APIs
- Prefer the generated client commands: `clusterdefaults`, `group`, `instanceinfo`, `wireguardpeer`
- The raw generated REST collections are `/clusterdefaultss`, `/groups`, `/instanceinfos`, and `/wireguardpeers`

## Template Context

Group templates are stored as plain text and rendered with these runtime values:
- Flat keys such as `hostname`, `local_hostname`, `instance_id`, `cluster_name`, `cloud_provider`, `region`, `nid`, `role`, `mac`, `ip`, `interfaces`, and `public_keys`
- Nested `vendor_data` matching the `/meta-data` payload
- Nested `meta_data` containing the full cloud-init metadata document
- Custom keys from `Group.Spec.MetaData`

The server validates templates at create and update time. A template must render successfully against sample metadata and produce valid YAML.

## Running With Real SMD

Set `SMD_URL` to use a real SMD instance.

Static auth mode (default when `tokensmith_url` is unset):
- Uses `SMD_JWT` (or `SMD_TOKEN`) as the outbound `Authorization: Bearer` token.

```bash
SMD_URL=https://smd.example.com \
SMD_JWT="$JWT" \
go run ./cmd/server/main.go serve --port 8888
```

TokenSmith dynamic auth mode (enabled when `tokensmith_url` is set):
- Exchanges `tokensmith_bootstrap_token` for a service token through `POST /oauth/token`.
- Refreshes tokens in the background and uses the dynamic token for all outbound SMD requests.
- Requires a bootstrap token from `tokensmith_bootstrap_token` (or `TOKENSMITH_BOOTSTRAP_TOKEN` fallback).
- If dynamic token retrieval/refresh fails, SMD requests fail closed (no silent fallback).

```bash
SMD_URL=https://smd.example.com \
TOKENSMITH_URL=https://tokensmith.example.com \
TOKENSMITH_BOOTSTRAP_TOKEN="$BOOTSTRAP_TOKEN" \
go run ./cmd/server/main.go serve --port 8888 \
  --tokensmith-target-service smd \
  --tokensmith-scopes metadata:read,groups:read \
  --tokensmith-refresh-skew-sec 300
```

TokenSmith server options:
- `tokensmith_url` / `TOKENSMITH_URL`
- `tokensmith_bootstrap_token` / `TOKENSMITH_BOOTSTRAP_TOKEN`
- `tokensmith_target_service` / `TOKENSMITH_TARGET_SERVICE` (default: `smd`)
- `tokensmith_scopes` / `TOKENSMITH_SCOPES` (diagnostics metadata only)
- `tokensmith_refresh_skew_sec` / `TOKENSMITH_REFRESH_SKEW_SEC` (default: `300`)

Request identity resolution prefers a WireGuard reverse lookup when available, then falls back to direct IP lookup through SMD.

## Optional WireGuard Support

Enable the userspace WireGuard controller by passing a CIDR whose host address is the server address inside the VPN network.

```bash
go run ./cmd/server/main.go serve --port 8888 --wireguard-server 100.97.0.1/16
```

Bootstrap a peer with:

```bash
curl \
	-X POST \
	-H "Content-Type: application/json" \
	-H "X-Forwarded-For: 10.252.0.26" \
	-d '{"public_key":"REPLACE_WITH_BASE64_WIREGUARD_PUBLIC_KEY"}' \
	http://localhost:8888/wg-init
```

If you also pass `--wireguard-only`, the server will reject requests whose remote address is not inside the configured WireGuard CIDR.

## Development

```bash
go mod tidy
make generate
make build
make test
make pre-commit-run
```

Additional examples live in [examples/README.md](examples/README.md).

## Release Notes

See [CHANGELOG.md](CHANGELOG.md) for the `0.1.0` release notes.

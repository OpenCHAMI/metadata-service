// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package handlers implements custom cloud-init datasource endpoints.

Endpoints:
  - /meta-data: node metadata in YAML
  - /user-data: default #cloud-config payload
  - /vendor-data: include-file list for group templates
  - /network-config: NoCloud network-config generated from SMD interfaces
  - /{group}.yaml: rendered group template (membership-gated)

Request flow:
1. Resolve requester IP (X-Forwarded-For first, then remote address)
2. Resolve component identity via SMD
3. Load cluster defaults + instance overrides
4. Merge runtime metadata context
5. Return endpoint-specific payload

Template rendering for /{group}.yaml:
1. Resolve node/component
2. Verify requested group membership from SMD
3. Load group template and metadata
4. Merge with runtime metadata context
5. Render via Pongo2

Template syntax and rendered YAML validity are validated in Group resource validation
at create/update time.

Metadata behavior:
  - instance-id defaults to component ID unless overridden by InstanceInfo
  - hostname/local-hostname are synthesized from cluster naming + component NID unless overridden
  - vendor data includes group metadata and network interface details when available

Common status behavior:
  - 404 for unresolved node/group membership misses
  - 500 for SMD/storage/template runtime failures

See CLOUDINIT.md for endpoint-level examples.
*/
package handlers

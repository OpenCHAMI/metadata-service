<!--
SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Legacy cloud-init service compatibility

This document compares the legacy OpenCHAMI cloud-init service with the current Fabrica-based metadata service to help sysadmins evaluate compatibility and migration steps.

## Summary
- **NoCloud endpoints**: Compatible. `/meta-data`, `/user-data`, `/vendor-data`, and `/{group}.yaml` are provided.
- **Client-side merge**: Compatible. Vendor-data returns `#include` list of group YAML files.
- **Group metadata visibility**: Compatible. All group memberships are present under `vendor_data.groups` in `/meta-data` and templates can reference them.
- **Admin API**: Different. Resource-style endpoints and payload shapes require updates.
- **Template storage**: Compatible. `templateEncoding: base64` is supported and decoded on create/update (stored as plain text).
- **Testing workflow**: Different. Uses mock SMD + `X-Forwarded-For` instead of impersonation routes.

## Endpoint comparison

| Purpose | Legacy | Current |
| --- | --- | --- |
| Meta-data | `/cloud-init/meta-data` | `/meta-data` |
| User-data | `/cloud-init/user-data` | `/user-data` |
| Vendor-data | `/cloud-init/vendor-data` | `/vendor-data` |
| Group YAML | `/cloud-init/{group}.yaml` | `/{group}.yaml` |
| Admin API | `/cloud-init/admin/...` | `/groups`, `/clusterdefaults`, `/instanceinfo` |

## Behavior parity details

### 1) Meta-data payload
**Legacy:** Returns `instance-id`, `hostname`, `local-hostname`, `cluster-name`, and `instance_data.v1` with `vendor_data` including `groups` metadata.

**Current:** Same structure. All group memberships are included in `instance_data.v1.vendor_data.groups`, even if a group has an empty template. Group templates can reference any variables present in `/meta-data`.

### 2) Vendor-data include list
**Legacy:** Returns `#include` list with URLs for each group YAML.

**Current:** Same behavior, but **filters out groups with empty templates** to avoid empty MIME parts (legacy issue #100 behavior retained).

### 3) Group YAML rendering
**Legacy:** Group templates can reference `vendor_data.groups["<group>"]` for any group metadata in the node’s memberships.

**Current:** Same. Templates receive a context with:
- `vendor_data.groups` for all memberships.
- Flattened keys (e.g., `hostname`, `instance_id`, `cluster_name`) for legacy-style access.
- Full meta-data exposed as `meta_data.*`.

### 4) Template encoding (base64)
**Legacy:** Allows base64-encoded template content with `file.encoding: base64`.

**Current:** Supports `spec.templateEncoding: base64` on create/update. Content is decoded and stored as plain text.

### 5) Hostname generation
**Legacy:** Short name defaults to `fmt.Sprintf("%.2s", clusterName)`.

**Current:** Uses `clusterName[:2]` when length >= 2; otherwise full string. For single-character cluster names this is slightly different.

### 6) IP and MAC fields
**Legacy:** `local_ipv4` can be string or object.

**Current:** `local_ipv4` is always a string.

## Admin API differences

### Groups
**Legacy create:**
```json
{
  "name": "x3001",
  "data": {"syslog_aggregator": "192.168.0.1"},
  "file": {"content": "#cloud-config\n...", "encoding": "plain"}
}
```

**Current create:**
```json
{
  "apiVersion": "cloud-init.openchami.io/v1",
  "kind": "Group",
  "metadata": {"name": "x3001"},
  "spec": {
    "description": "Cabinet x3001",
    "template": "#cloud-config\n...",
    "metaData": {"syslog_aggregator": "192.168.0.1"}
  }
}
```

### Cluster defaults and instance info
- **Legacy:** `/admin/cluster-defaults` and `/admin/instance-info/{id}`
- **Current:** `/clusterdefaults` and `/instanceinfo` with resource-style payloads

## Migration checklist
1. Update admin scripts to use resource-style endpoints and payloads.
2. If sending base64 templates, set `spec.templateEncoding: base64` (the server stores decoded plain text).
3. Confirm templates reference `vendor_data.groups` or `meta_data` keys as needed.
4. If relying on impersonation routes, switch to `X-Forwarded-For` with mock SMD for testing.
5. Validate that any client expecting complex `local_ipv4` handles a string instead.

## References
- Legacy repo: https://github.com/OpenCHAMI/cloud-init
- NoCloud datasource docs: https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html

<!--
SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC

SPDX-License-Identifier: MIT
-->

# Legacy Cloud-Init Compatibility

This document compares the legacy OpenCHAMI cloud-init service with the current Fabrica-based metadata service.

## Summary

- NoCloud metadata compatibility is present for `/meta-data`, `/user-data`, `/vendor-data`, and `/{group}.yaml`.
- The current service also exposes `/network-config`, `/health`, `/openapi.json`, and `/docs`.
- Group membership metadata remains available under `instance_data.v1.vendor_data.groups` in `/meta-data`.
- The admin and resource management APIs are different and require updated tooling.
- Templates are stored as plain text in the current API.
- Development testing uses mock SMD data plus `X-Forwarded-For` rather than impersonation routes.

## Endpoint Comparison

| Purpose | Legacy | Current |
| --- | --- | --- |
| Meta-data | `/cloud-init/meta-data` | `/meta-data` |
| User-data | `/cloud-init/user-data` | `/user-data` |
| Vendor-data | `/cloud-init/vendor-data` | `/vendor-data` |
| Network-config | not provided | `/network-config` |
| Group YAML | `/cloud-init/{group}.yaml` | `/{group}.yaml` |
| Health | not provided | `/health` |
| OpenAPI JSON | not provided | `/openapi.json` |
| Swagger UI | not provided | `/docs` |
| Admin API | `/cloud-init/admin/...` | generated resource collections |

## Behavior Notes

### Meta-data payload

Legacy behavior
- Returns the cloud-init metadata document with `instance_data.v1.vendor_data.groups` populated from group membership.

Current behavior
- Preserves the same high-level structure.
- Includes all group memberships in `vendor_data.groups`, even when a group has no renderable template.
- Prefers a WireGuard reverse lookup for request identity when the client IP is a VPN address.
- Picks HMN interface data first when deciding the boot IP and MAC for multi-NIC nodes.

### Vendor-data include list

Legacy behavior
- Returns a `#include` list of group YAML URLs.

Current behavior
- Returns the same `#include` style payload.
- Filters out groups whose stored template is empty.

### Group template rendering

Legacy behavior
- Templates can rely on group metadata and node metadata at render time.

Current behavior
- Templates receive flat keys such as `hostname`, `instance_id`, `cluster_name`, `nid`, `role`, `mac`, `ip`, `interfaces`, and `public_keys`.
- Templates also receive nested `vendor_data` and nested `meta_data` objects.
- Custom group metadata is exposed from `Group.Spec.MetaData`.
- Templates must render successfully and produce valid YAML at create or update time.

### Template storage

Legacy behavior
- Group data could carry encoded file payloads.

Current behavior
- `Group.Spec.Template` is plain text.
- There is no `templateEncoding` field in the current API.

### Hostname generation

Legacy behavior
- Short-name defaults were derived from the cluster name.

Current behavior
- If `short_name` is unset, the hostname prefix uses the first two characters of `cluster_name` when available, otherwise the full short cluster name.

### IP and MAC lookup

Legacy behavior
- Boot network information was supplied by the older service path.

Current behavior
- IP and MAC resolution can fall back to top-level component fields when Ethernet interface data is absent.

## Resource Management API Differences

Prefer the generated client commands over the raw collections. The current raw collection paths are:
- `/clusterdefaultss`
- `/groups`
- `/instanceinfos`
- `/wireguardpeers`

The generated client commands are:
- `clusterdefaults`
- `group`
- `instanceinfo`
- `wireguardpeer`

Current create and update requests require both `metadata` and `spec` fields.

Example create request for a group:

```json
{
  "metadata": {
    "name": "compute"
  },
  "spec": {
    "description": "Compute nodes",
    "template": "#cloud-config\nhostname: {{ hostname }}\n",
    "metaData": {
      "scheduler": "slurm"
    }
  }
}
```

## Migration Checklist

1. Update admin scripts to use the generated client or the current resource collection paths.
2. Convert any legacy encoded template workflow to plain-text `spec.template` payloads.
3. Validate templates against the current runtime context, including `vendor_data` and `meta_data`.
4. Replace impersonation-based testing with mock SMD requests using `X-Forwarded-For`.
5. If you rely on raw REST instead of the generated client, confirm your tooling targets `/clusterdefaultss`, `/groups`, `/instanceinfos`, and `/wireguardpeers`.

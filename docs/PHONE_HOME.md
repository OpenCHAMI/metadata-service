<!--
SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
SPDX-License-Identifier: MIT
-->

# Phone-home reports and WireGuard shutdown notifications

`POST /phone-home/{id}` accepts reports regardless of WireGuard configuration. It
is a bootstrap reporting endpoint and does not require an operator JWT. It stores
no event history and exposes no status/query API. Configure network exposure for
your site: an anonymous receipt is advisory, never an authenticated readiness or
provisioning-success assertion. Cloud-init normally phones home once per instance,
not on every reboot.

## HTTP behavior

- Empty POSTs remain accepted. Nonempty bodies must be
  `application/x-www-form-urlencoded`, as sent by cloud-init's phone_home module.
- The body limit is 16 KiB. Only `instance_id`, `hostname`, and `fqdn` are selected
  for notification data, each at most 256 bytes, without CR/LF/NUL. Duplicate
  values for those fields are rejected. SSH keys and unknown fields are not emitted.
- IDs are 1–256 ASCII letters, digits, dots, underscores, colons, or hyphens. Treat
  peer UIDs as opaque; both existing peer IDs and TypeIDs fit this format.
- Valid report-only calls return 200. Malformed fields/IDs return 400, oversized
  bodies 413, and unsupported nonempty body types 415.
- With `wireguard_only: true`, phone-home remains inside the restricted integration
  group and outside transport peers receive 403. Public health/docs routes retain
  their separate policy. Missing/failed controller setup fails startup in this mode.

Example cloud-config:

```yaml
phone_home:
  url: http://metadata.example/phone-home/node-1
  post: [instance_id, hostname, fqdn]
  tries: 10
```

The URL ID and payload identity values are caller claims. A receipt does not prove
that these claims are true or that all cloud-init modules succeeded. Site automation
must not turn an anonymous notification into enrollment, credentials, or authoritative
node state. An instance ID also does not establish a new provisioning generation.

## WireGuard cleanup boundary

With a controller configured, a callback may request cleanup only when its original
TCP peer is in the tunnel network and matches both the stored intent and active
controller peer's exact assigned IP. A stored intent alone does not authorize cleanup.
The path can name a stored peer UID or a component ID whose SMD WireGuard IP resolves
to that peer. A different tunnel peer targeting it receives 403. Unknown IDs are
report-only and do not fall back to deleting the caller's peer.

Physical-network and reverse-proxy callbacks are report-only, even if they provide
WireGuard-looking `X-Forwarded-For`, `Forwarded`, or `X-Real-IP` headers. Cleanup
must reach the service directly over the tunnel. WireGuard source-network routing
and firewall isolation remain deployment responsibilities: a subnet check alone
is not cryptographic node attestation. No forwarding-header trust is added here.

The service removes the peer intent and queues its existing lifecycle deletion
for reconciliation. If enqueue fails, it attempts to restore the intent and returns
503 so the callback can retry. A 200 is not confirmation that the controller has
removed the tunnel. Repeated callbacks after cleanup are safe and produce new
receipt events without a new removal request.

## Event contract (version 1)

Notifications use a complete CloudEvents JSON envelope per stdout line, without a
log timestamp prefix. Standard fields include `specversion`, random `id`, server
`time`, `type`, `source` (`/metadata-service`), `subject`, and JSON `data`. The same
ID is used when offering the event to Fabrica's existing in-process bus.

| Type | Subject | Data |
| --- | --- | --- |
| `org.openchami.metadata.phonehome.received.v1` | `nodes/{path-id}` | `node_id` (claimed path ID), optional claimed `instance_id`, `hostname`, `fqdn`, observed `transport_ip`, `identity_basis`, optional `request_id`, and matched `peer_uid` |
| `org.openchami.metadata.wireguard.shutdown.v1` | `wireguardpeers/{uid}` | `peer_uid`, `reason`, `outcome`, and optional `request_id` / `phone_home_event_id` |

Receipt `identity_basis` is `reported` or `wireguard_address`. The latter means the
transport address matched the stored peer's assignment; payload fields remain
unverified claims. It is not a stronger attestation protocol.

Shutdown `outcome` is emitted after the removal decision:

- `removed`: the controller's removal operation succeeded.
- `already_absent`: the controller reported that the peer did not exist; no new
  tunnel removal is claimed.
- `failed`: controller removal failed. Raw device errors and key material are not
  included in the notification; operational diagnostics stay in service logs.
- `skipped_controller_disabled`: no controller was available to perform removal.

The reason is `phone_home` for correlated callbacks, otherwise `resource_deleted`
for deletion through the existing resource/reconciliation path. Receipt IDs and
request IDs link callback and shutdown result. Domain notifications have no resource
lifecycle extensions and do not trigger another reconciliation/removal cycle.

The existing `phoneHomeID` metadata is preserved on the lifecycle event.
The existing `io.fabrica.wireguardpeer.deleted` lifecycle notification remains the
reconciliation trigger. It signifies intent deletion, not completed tunnel shutdown.
Consumers should use the new outcome event when they need that distinction.

## Collection and delivery

Use the host/container logging pipeline to collect JSON notifications from stdout:
redirect to a log file with site-owned rotation/permissions, retain them in journald,
or forward with the site's syslog agent. Ordinary diagnostics can still appear in
the process logs; filter by the CloudEvent `type`. No collector, syslog destination,
S3 credentials, Parquet compactor, or database is a startup dependency. A collector
may later route these same events into a site-selected archive or analytics tool.

Emission is best effort, not exactly-once or durable. Duplicate callbacks emit new
events. The in-memory bus may reject an enqueue and loses pending events on restart;
stdout acceptance does not imply disk sync or downstream retention. Output failures
are reported in operational logs. General receipt notification failures do not turn
a report-only callback into a provisioning failure. WireGuard lifecycle enqueue
failures receive the separate retry behavior above. Failed controller cleanup is
reported, but this change does not add a persistent retry queue.

## Upgrade, validation, and rollback

This adds an anonymous reporting route where no-WireGuard deployments previously
returned 404. It also deliberately stops physical/proxied or cross-peer requests
from removing a tunnel. Before upgrading, direct cleanup callbacks to the service's
WireGuard address and verify route/firewall isolation. Do not solve a failed proxy
cleanup test by trusting arbitrary client headers. Full trusted-ingress policy and
operator API authentication remain separate work.

Validate an ordinary form POST without WireGuard, its stdout receipt, and duplicate
callbacks. With WireGuard, validate own-peer removal and the correlated `removed`
event, another-peer 403, report-only outside/proxied requests, and WireGuard-only
403s for outside callers with forged headers. Exercise output/queue failures and
restart behavior with your collector; no delivery guarantee should be inferred
from a callback alone.

No receipt store/schema migration is introduced. Reverting the binary removes the
new reports/events and restores the old registration and weaker cleanup behavior;
retain any site containment needed during rollback.

# Metadata-service authorization (Casbin RBAC) – operational guide

This document describes how to **enable**, **verify**, and **roll back** Casbin-based authorization (authz) in `metadata-service`.

It is intentionally operational (deployment-focused). For route classification and objects/actions, see:
- `docs/authz_route_inventory.md`
- `policies/metadata-service/policy.csv`

## What changes when authz is enabled

- **Authentication** still uses `tokensmith` JWT validation.
- **Authorization** is enforced by Casbin checks for **Protected** endpoints.
- **Public** endpoints (cloud-init NoCloud routes, OpenAPI docs) remain unauthenticated.

Protected resources in this service:
- Node metadata records (InstanceInfo, ClusterDefaults)
- Group metadata (Group)
- WireGuard peer management (WireGuardPeer + `/wg-init`, `/phone-home/{id}`)

## RBAC model (minimum roles)

Roles used across OpenCHAMI services:

- `admin`: full CRUD
- `operator`: read + write, **no delete**
- `viewer`: read-only
- `service`: service-to-service calls (read-only for this service)

In metadata-service, the policy fragment defines two objects:

- `node-metadata`
- `group-metadata`

Actions are:

- `read` (GET/HEAD)
- `write` (POST/PUT/PATCH)
- `delete` (DELETE)

## Service-to-service expectations

`metadata-service` is typically called by:

- **Nodes** during early boot via NoCloud endpoints (`/meta-data`, `/{group}.yaml`, ...). These are **Public** by design.
- **Operators/admins** using the generated CRUD API and/or client.
- **Other services** (e.g. boot-service) for administrative reads in some workflows.

If boot-service (or any other service) needs to call **Protected** endpoints, it must present a JWT that maps to the `service` role (read-only in this service).

A common deployment pattern is to map an explicit service principal to the `service` role using a grouping rule overlay, for example:

```csv
# overlay policy (deployment-specific)
g, svc:boot-service, service
```

## Enablement

### 1) Ensure a policy is available

This repo provides a **policy fragment**:

- `policies/metadata-service/policy.csv`

Your deployment must make this available to tokensmith’s policy loader (exact mechanism depends on how tokensmith is deployed).

Recommended approach:
- Mount a policies directory into the tokensmith container (or wherever the policy loader runs).
- Include the fragment at a stable path (e.g. `/policies/metadata-service/policy.csv`).

### 2) Configure authz mode

metadata-service relies on `tokensmith` for the authz middleware. Deployments should set the same enablement knobs used by the tokensmith integration in this service.

Operationally, you should have three supported modes:

- `disabled` (default): only authentication is applied; no Casbin enforcement.
- `shadow`: Casbin is evaluated, denials are recorded/metric’d, but requests are allowed.
- `enforce`: Casbin denials return HTTP 403.

If your deployment uses environment variables to set this, ensure they are applied to the metadata-service server process.

### 3) Restart / roll

- Restart the metadata-service deployment after enabling authz and mounting policies.
- Confirm the process starts cleanly (see “Verify” below).

## Verify

### A) Sanity check: service is up

```bash
curl -sS http://$METADATA_SVC/openapi.json | head
```

### B) Verify **Public** endpoints remain reachable without a JWT

```bash
curl -sS -H 'X-Forwarded-For: 10.0.0.100' http://$METADATA_SVC/meta-data
curl -sS -H 'X-Forwarded-For: 10.0.0.100' http://$METADATA_SVC/vendor-data
```

(These routes are intentionally unauthenticated for early-boot nodes.)

### C) Verify **Protected** endpoints require authorization

Pick a protected endpoint, e.g. list groups:

```bash
# Expect 401 if you omit JWT entirely (authentication required)
curl -i http://$METADATA_SVC/groups/

# With a JWT:
# - viewer/service should be allowed (read)
# - operator/admin should be allowed (read)
# - unknown/unauthorized principals should get 403 in enforce mode
curl -i -H "Authorization: Bearer $JWT" http://$METADATA_SVC/groups/
```

To verify operator vs admin delete behavior:

```bash
# operator should be denied delete in enforce mode
curl -i -X DELETE -H "Authorization: Bearer $OPERATOR_JWT" http://$METADATA_SVC/groups/$GROUP_UID/

# admin should be allowed
curl -i -X DELETE -H "Authorization: Bearer $ADMIN_JWT" http://$METADATA_SVC/groups/$GROUP_UID/
```

### D) Verify service-to-service role

If boot-service uses a service JWT mapped to `service`, verify it can read but not write:

```bash
# read ok
curl -i -H "Authorization: Bearer $BOOT_SVC_JWT" http://$METADATA_SVC/instanceinfos/

# write should be denied
curl -i -X POST -H "Authorization: Bearer $BOOT_SVC_JWT" \
  -H 'Content-Type: application/json' \
  -d '{"name":"x0c0s0b0n0","hostname":"x0c0s0b0n0"}' \
  http://$METADATA_SVC/instanceinfos/
```

### E) Observe logs/metrics

In `shadow` mode you should see deny decisions recorded while requests still succeed.

In `enforce` mode you should see HTTP 403 on denied access.

If your deployment scrapes Prometheus metrics, look for tokensmith authz decision counters (exact metric names depend on the tokensmith implementation).

## Rollback

Rollback should be safe and fast.

1. Switch authz mode back to `disabled`.
2. (Optional) Remove/unmount the policies directory.
3. Restart the metadata-service deployment.

Behavior after rollback:
- Protected endpoints continue to require authentication (JWT), but will no longer enforce role-based authorization.
- Public endpoints remain unchanged.

## Troubleshooting

### Policy load failure

If authz is enabled and the policy cannot be loaded, recommended behavior is **fail fast** (process exits) rather than silently running without authorization.

Check:
- the policy directory mount
- file permissions
- the configured policy path / glob

### Unexpected 403

Confirm:
- the JWT validates (signature, issuer, audience)
- the principal maps to the expected role(s)
- the request’s object/action mapping matches the policy (see `docs/authz_route_inventory.md` and `policies/metadata-service/policy.csv`)

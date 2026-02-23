# Metadata Service (cloud-init) – Authorization route inventory

Scope: this document inventories **all HTTP routes registered by metadata-service** and classifies each route as **Protected** (requires authentication + Casbin authorization) or **Public** (no authz check). Public routes include explicit justification.

This service currently contains:
- Fabrica-generated CRUD APIs for Kubernetes-style resources (ClusterDefaults, Group, InstanceInfo, WireGuardPeer)
- Custom cloud-init metadata server endpoints (NoCloud-style)
- Custom WireGuard bootstrap endpoints
- OpenAPI documentation endpoints

Terminology (for later RBAC enforcement):
- **Subject**: authenticated principal from JWT (user/service)
- **Resource**: logical API/resource being accessed (e.g., `groups`, `instanceinfos`)
- **Action**: CRUD-ish verb (e.g., `read`, `create`, `update`, `patch`, `delete`)

## Summary

- **Public (unauthenticated)**: cloud-init metadata endpoints (by design for node bootstrapping), and documentation endpoints.
- **Protected**: all resource CRUD APIs, and WireGuard bootstrap endpoints.

> Notes
> - Public cloud-init endpoints can indirectly expose sensitive data (e.g., SSH keys, user-data). In typical HPC usage these endpoints are only reachable from a management/provisioning network. They are classified Public because cloud-init clients often cannot present a JWT during early boot.
> - WireGuard endpoints are classified Protected because they grant network access. If a deployment requires unauthenticated wireguard bootstrap, that should be an explicit opt-in in the future; default should be Protected.

## Route inventory and classification

### 1) Fabrica-generated resource APIs (Protected)

Source: `cmd/server/routes_generated.go`

These are administrative APIs for creating/updating/deleting metadata records. They should not be callable anonymously.

#### ClusterDefaults (`/clusterdefaultss`) – Protected

| Method | Route | Classification | Justification / Notes |
|---|---|---|---|
| GET | `/clusterdefaultss/` | Protected | Reads cluster-wide defaults used to render metadata; should require at least viewer role. |
| POST | `/clusterdefaultss/` | Protected | Creates defaults; admin/operator only. |
| GET | `/clusterdefaultss/{uid}/` | Protected | Reads a specific defaults object. |
| PUT | `/clusterdefaultss/{uid}/` | Protected | Updates spec; admin/operator only. |
| PATCH | `/clusterdefaultss/{uid}/` | Protected | Updates spec; admin/operator only. |
| DELETE | `/clusterdefaultss/{uid}/` | Protected | Deletes; admin only (operator explicitly has no delete). |
| PUT | `/clusterdefaultss/{uid}/status/` | Protected | Updates status; typically controller/internal use. |
| PATCH | `/clusterdefaultss/{uid}/status/` | Protected | Updates status; typically controller/internal use. |

#### Group (`/groups`) – Protected

| Method | Route | Classification | Justification / Notes |
|---|---|---|---|
| GET | `/groups/` | Protected | Lists group metadata. |
| POST | `/groups/` | Protected | Creates group metadata. |
| GET | `/groups/{uid}/` | Protected | Reads group metadata. |
| PUT | `/groups/{uid}/` | Protected | Updates group metadata. |
| PATCH | `/groups/{uid}/` | Protected | Updates group metadata. |
| DELETE | `/groups/{uid}/` | Protected | Deletes group metadata; admin only. |
| PUT | `/groups/{uid}/status/` | Protected | Status update; controller/internal use. |
| PATCH | `/groups/{uid}/status/` | Protected | Status update; controller/internal use. |

#### InstanceInfo (`/instanceinfos`) – Protected

| Method | Route | Classification | Justification / Notes |
|---|---|---|---|
| GET | `/instanceinfos/` | Protected | Lists instance metadata records. |
| POST | `/instanceinfos/` | Protected | Creates instance metadata record. |
| GET | `/instanceinfos/{uid}/` | Protected | Reads instance metadata record. |
| PUT | `/instanceinfos/{uid}/` | Protected | Updates instance metadata record. |
| PATCH | `/instanceinfos/{uid}/` | Protected | Updates instance metadata record. |
| DELETE | `/instanceinfos/{uid}/` | Protected | Deletes; admin only. |
| PUT | `/instanceinfos/{uid}/status/` | Protected | Status update; controller/internal use. |
| PATCH | `/instanceinfos/{uid}/status/` | Protected | Status update; controller/internal use. |

#### WireGuardPeer (`/wireguardpeers`) – Protected

| Method | Route | Classification | Justification / Notes |
|---|---|---|---|
| GET | `/wireguardpeers/` | Protected | Lists WireGuard peer records; sensitive. |
| POST | `/wireguardpeers/` | Protected | Creates peer record; admin/operator only. |
| GET | `/wireguardpeers/{uid}/` | Protected | Reads a peer record; sensitive. |
| PUT | `/wireguardpeers/{uid}/` | Protected | Updates peer record; sensitive. |
| PATCH | `/wireguardpeers/{uid}/` | Protected | Updates peer record; sensitive. |
| DELETE | `/wireguardpeers/{uid}/` | Protected | Deletes peer record; admin only. |
| PUT | `/wireguardpeers/{uid}/status/` | Protected | Status update; controller/internal use. |
| PATCH | `/wireguardpeers/{uid}/status/` | Protected | Status update; controller/internal use. |

### 2) Cloud-init metadata server endpoints (Public)

Source: `cmd/server/cloudinit_routes.go`

These endpoints are queried by cloud-init/booting nodes. In many deployments, a node does not have a JWT available during early boot, so these must remain reachable without standard user authentication.

| Method | Route | Classification | Public justification |
|---|---|---|---|
| GET | `/meta-data` | Public | Standard cloud-init metadata endpoint; needed during provisioning/bootstrapping when node lacks credentials. |
| GET | `/user-data` | Public | Standard cloud-init endpoint. Often contains bootstrap config; exposure risk mitigated by network isolation. |
| GET | `/vendor-data` | Public | Standard cloud-init endpoint; used for vendor-specific configuration. |
| GET | `/network-config` | Public | Standard cloud-init endpoint for network configuration. |
| GET | `/{group}.yaml` | Public | Convenience endpoint to fetch per-group user-data; used by cloud-init to render node configuration. |

#### Service-to-service callouts

These cloud-init handlers may call out to SMD (via `pkg/smdclient`) to map IPs/IDs and fetch additional data.

Boot-service is **not expected** to call these endpoints directly in the normal workflow; they are primarily **node-to-metadata-service**.

### 3) WireGuard bootstrap endpoints (Protected)

Source: `cmd/server/wireguard_routes.go`

These endpoints create/remove WireGuard peers and can change a node’s network reachability. They should require authentication + authorization.

| Method | Route | Classification | Justification / Notes |
|---|---|---|---|
| POST | `/wg-init` | Protected | Allocates WireGuard tunnel/peer; grants network access. |
| POST | `/phone-home/{id}` | Protected | Removes peer; should not be anonymous. |

#### Service-to-service callouts

These endpoints optionally call SMD to associate WireGuard IPs with nodes (`smd.AddWGIP`, etc.).

Boot-service is **not expected** to call these endpoints.

### 4) OpenAPI/Swagger documentation endpoints (Public)

Source: `cmd/server/routes_generated.go`, `cmd/server/openapi_generated.go`

| Method | Route | Classification | Public justification |
|---|---|---|---|
| GET | `/openapi.json` | Public | Documentation/spec endpoint; does not expose live data. |
| GET | `/docs` | Public | Swagger UI static HTML for developer/operator convenience. |

## Gaps / follow-ups for later steps

- Decide whether cloud-init endpoints should support **optional authentication** (if JWT presented, apply authorization) while still allowing unauthenticated bootstrapping. This would enable tightening access in environments where nodes can present a token.
- Consider additional public health endpoints if present (none discovered in current route registration files).

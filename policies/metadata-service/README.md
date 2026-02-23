# metadata-service authorization policy fragment

This directory contains the **metadata-service Casbin policy fragment** to be consumed by the shared `tokensmith` authorization middleware.

## Files

- `policy.csv`: RBAC policy rules for metadata-service resources.

## Objects and actions

Objects (`obj`) are service-scoped strings.

- `node-metadata`
- `group-metadata`

Actions (`act`):

- `read` (GET/HEAD)
- `write` (POST/PUT/PATCH)
- `delete` (DELETE)

These must align with the route annotations in `internal/authz` and the route inventory in `docs/authz_route_inventory.md`.

## Roles

Minimum roles used across OpenCHAMI services:

- `admin`: full CRUD
- `operator`: read+write, no delete
- `viewer`: read-only
- `service`: service-to-service calls (read-only for this service)

## Role mapping / JWT claim expectations (notes)

tokensmith is expected to:

1. Validate the JWT.
2. Determine a **subject** string for authorization.
3. Map that subject to a role (or provide roles directly) and call Casbin.

Recommended conventions (document and keep consistent across services):

- Use a string subject in Casbin that is either:
  - a role directly (`admin`, `operator`, ...), or
  - a principal identifier (e.g. `user:alice`, `svc:boot-service`) with grouping rules (`g, svc:boot-service, service`).

Typical JWT claim sources:

- A role list claim such as `roles` or `groups`.
- `sub` for principal id.
- `azp` / `client_id` for service identity when using an external IdP.

### Service-to-service example

For boot-service reading metadata-service, ensure the calling JWT maps to the `service` role.

If you use explicit principal mapping, add grouping rules in a deployment overlay file (not checked into this repo), for example:

```csv
# deployment overlay policy
g, svc:boot-service, service
```

## Mounting / loading

This repo only provides the fragment. A deployment should mount or bundle it into the tokensmith policy load path (exact mechanism depends on tokensmith implementation).

Example Kubernetes-style mount:

- Mount `policies/metadata-service/policy.csv` into the tokensmith container at something like `/policies/metadata-service/policy.csv`.
- Configure tokensmith to load all `*.csv` under `/policies/**`.

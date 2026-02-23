// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package authz

// Object names (Casbin "obj") used by metadata-service.
//
// These strings must align with policy definitions maintained in tokensmith and
// the route inventory in docs/authz_route_inventory.md.
const (
	ObjectNodeMetadata  = "node-metadata"
	ObjectGroupMetadata = "group-metadata"
)

// Action names (Casbin "act").
//
// Convention: HTTP verbs map to actions as follows:
//   - GET/HEAD -> read
//   - POST/PUT/PATCH -> write
//   - DELETE -> delete
const (
	ActionRead   = "read"
	ActionWrite  = "write"
	ActionDelete = "delete"
)

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package authz

import "net/http"

// ExpectedRoutes is the deny-by-default coverage list.
//
// IMPORTANT: Keep this list in sync with docs/authz_route_inventory.md.
// Any newly added route must be explicitly classified as Public or Protected by
// wrapping it with authz.Public()/authz.Require() AND by adding it to this list.
func ExpectedRoutes() []RouteSpec {
	return []RouteSpec{
		// Fabrica-generated resource APIs (cmd/server/routes_generated.go)
		{Method: http.MethodGet, Path: "/clusterdefaultss/"},
		{Method: http.MethodPost, Path: "/clusterdefaultss/"},
		{Method: http.MethodGet, Path: "/clusterdefaultss/{uid}/"},
		{Method: http.MethodPut, Path: "/clusterdefaultss/{uid}/"},
		{Method: http.MethodPatch, Path: "/clusterdefaultss/{uid}/"},
		{Method: http.MethodDelete, Path: "/clusterdefaultss/{uid}/"},
		{Method: http.MethodPut, Path: "/clusterdefaultss/{uid}/status/"},
		{Method: http.MethodPatch, Path: "/clusterdefaultss/{uid}/status/"},

		{Method: http.MethodGet, Path: "/groups/"},
		{Method: http.MethodPost, Path: "/groups/"},
		{Method: http.MethodGet, Path: "/groups/{uid}/"},
		{Method: http.MethodPut, Path: "/groups/{uid}/"},
		{Method: http.MethodPatch, Path: "/groups/{uid}/"},
		{Method: http.MethodDelete, Path: "/groups/{uid}/"},
		{Method: http.MethodPut, Path: "/groups/{uid}/status/"},
		{Method: http.MethodPatch, Path: "/groups/{uid}/status/"},

		{Method: http.MethodGet, Path: "/instanceinfos/"},
		{Method: http.MethodPost, Path: "/instanceinfos/"},
		{Method: http.MethodGet, Path: "/instanceinfos/{uid}/"},
		{Method: http.MethodPut, Path: "/instanceinfos/{uid}/"},
		{Method: http.MethodPatch, Path: "/instanceinfos/{uid}/"},
		{Method: http.MethodDelete, Path: "/instanceinfos/{uid}/"},
		{Method: http.MethodPut, Path: "/instanceinfos/{uid}/status/"},
		{Method: http.MethodPatch, Path: "/instanceinfos/{uid}/status/"},

		{Method: http.MethodGet, Path: "/wireguardpeers/"},
		{Method: http.MethodPost, Path: "/wireguardpeers/"},
		{Method: http.MethodGet, Path: "/wireguardpeers/{uid}/"},
		{Method: http.MethodPut, Path: "/wireguardpeers/{uid}/"},
		{Method: http.MethodPatch, Path: "/wireguardpeers/{uid}/"},
		{Method: http.MethodDelete, Path: "/wireguardpeers/{uid}/"},
		{Method: http.MethodPut, Path: "/wireguardpeers/{uid}/status/"},
		{Method: http.MethodPatch, Path: "/wireguardpeers/{uid}/status/"},

		{Method: http.MethodGet, Path: "/openapi.json"},
		{Method: http.MethodGet, Path: "/docs"},

		// Health (cmd/server/main.go)
		{Method: http.MethodGet, Path: "/health"},

		// Cloud-init metadata server endpoints (cmd/server/cloudinit_routes.go)
		{Method: http.MethodGet, Path: "/meta-data"},
		{Method: http.MethodGet, Path: "/user-data"},
		{Method: http.MethodGet, Path: "/vendor-data"},
		{Method: http.MethodGet, Path: "/network-config"},
		{Method: http.MethodGet, Path: "/{group}.yaml"},

		// WireGuard bootstrap endpoints (cmd/server/wireguard_routes.go)
		{Method: http.MethodPost, Path: "/wg-init"},
		{Method: http.MethodPost, Path: "/phone-home/{id}"},
	}
}

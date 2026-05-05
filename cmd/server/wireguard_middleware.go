// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package main

import (
	"net"
	"net/http"

	"github.com/OpenCHAMI/metadata-service/pkg/wireguard"
)

// wireGuardOnlyMiddleware rejects requests whose client IP is not within the WireGuard network when enabled.
func wireGuardOnlyMiddleware(controller *wireguard.Controller) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if controller == nil {
				next.ServeHTTP(w, r)
				return
			}
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			ip := net.ParseIP(host)
			if ip == nil || !controller.Network.Contains(ip) {
				http.Error(w, "wireguard-only access required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

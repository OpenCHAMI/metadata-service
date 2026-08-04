// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package main

import (
	"context"
	"net/http"

	"github.com/openchami/metadata-service/pkg/wireguard"
)

// wireGuardControllerMiddleware injects the controller into the request context for reconciliation hooks.
func wireGuardControllerMiddleware(controller *wireguard.Controller) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if controller == nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), wireguard.ControllerContextKey, controller)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

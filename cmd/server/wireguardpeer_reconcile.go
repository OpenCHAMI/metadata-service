// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package main

import (
	"context"
	"fmt"
	"net/http"

	cloudinitv1 "github.com/OpenCHAMI/cloud-init/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
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

func controllerFromCtx(ctx context.Context) *wireguard.Controller {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(wireguard.ControllerContextKey); v != nil {
		if c, ok := v.(*wireguard.Controller); ok {
			return c
		}
	}
	return nil
}

func reconcileWireGuardPeer(ctx context.Context, peer *cloudinitv1.WireGuardPeer) error {
	controller := controllerFromCtx(ctx)
	if controller == nil {
		return nil
	}
	if peer.Spec.PublicKey == "" {
		return fmt.Errorf("public_key is required for reconciliation")
	}
	if peer.Spec.AllowedIP == "" {
		return fmt.Errorf("allowed_ip is required for reconciliation")
	}
	return controller.UpsertPeer(peer.GetUID(), peer.Spec.PublicKey, peer.Spec.AllowedIP)
}

func removeWireGuardPeer(ctx context.Context, uid string) error {
	controller := controllerFromCtx(ctx)
	if controller == nil {
		return nil
	}
	return controller.RemovePeerByID(uid)
}

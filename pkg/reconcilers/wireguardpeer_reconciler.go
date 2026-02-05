// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT
// This file contains user-customizable reconciliation logic for WireGuardPeer.
//
// ⚠️ This file is safe to edit - it will NOT be overwritten by code generation.
package reconcilers

import (
	"context"
	"fmt"
	"sync"

	v1 "github.com/OpenCHAMI/cloud-init/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
)

var (
	wireGuardControllerMu sync.RWMutex
	wireGuardController   *wireguard.Controller
)

// SetWireGuardController configures the userspace WireGuard controller for reconciliation.
func SetWireGuardController(controller *wireguard.Controller) {
	wireGuardControllerMu.Lock()
	defer wireGuardControllerMu.Unlock()
	wireGuardController = controller
}

func getWireGuardController() *wireguard.Controller {
	wireGuardControllerMu.RLock()
	defer wireGuardControllerMu.RUnlock()
	return wireGuardController
}

// reconcileWireGuardPeer contains custom reconciliation logic.
//
// This method is called by the generated Reconcile() orchestration method.
// Implement WireGuardPeer-specific reconciliation logic here.
//
// Guidelines:
//  1. Keep this method idempotent (safe to call multiple times)
//  2. Update Status fields to reflect observed state
//  3. Emit events for significant state changes using r.EmitEvent()
//  4. Use r.Logger for debugging (Infof, Warnf, Errorf, Debugf)
//  5. Return errors for transient failures (will retry with backoff)
//  6. Access storage via r.Client (Get, List, Update, Create, Delete)
//
// Example implementation patterns:
//
// For hardware resources (BMC, Node):
//   - Connect to hardware endpoint
//   - Query current state
//   - Update Status.Connected, Status.Version, Status.Health
//   - Emit events when state changes
//
// For hierarchical resources (Rack, Chassis):
//   - Create/reconcile child resources
//   - Update Status with child counts and references
//   - Emit events when topology changes
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - res: The WireGuardPeer resource to reconcile
//
// Returns:
//   - error: If reconciliation failed (will trigger retry with backoff)
func (r *WireGuardPeerReconciler) reconcileWireGuardPeer(ctx context.Context, res *v1.WireGuardPeer) error {
	controller := getWireGuardController()
	if controller == nil {
		res.Status.Ready = false
		res.Status.Phase = "Disabled"
		res.Status.Message = "wireguard controller not configured"
		r.Logger.Warnf("WireGuard controller not configured; skipping reconciliation for %s", res.GetUID())
		return nil
	}
	if res.Spec.PublicKey == "" {
		return fmt.Errorf("public_key is required for reconciliation")
	}
	if res.Spec.AllowedIP == "" {
		return fmt.Errorf("allowed_ip is required for reconciliation")
	}
	if err := controller.UpsertPeer(res.GetUID(), res.Spec.PublicKey, res.Spec.AllowedIP); err != nil {
		return err
	}

	res.Status.Ready = true
	res.Status.Phase = "Configured"
	res.Status.Message = "peer configured"
	return nil
}

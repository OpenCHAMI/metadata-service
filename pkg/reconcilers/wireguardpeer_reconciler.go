// Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
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
	wgControllerMu     sync.RWMutex
	activeWGController *wireguard.Controller
)

// SetWireGuardController injects the WireGuard controller used by the reconciler.
// Call this before the reconciliation runtime starts, from startReconciliationRuntime.
func SetWireGuardController(c *wireguard.Controller) {
	wgControllerMu.Lock()
	defer wgControllerMu.Unlock()
	activeWGController = c
}

func getActiveWGController() *wireguard.Controller {
	wgControllerMu.RLock()
	defer wgControllerMu.RUnlock()
	return activeWGController
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
	c := getActiveWGController()
	if c == nil {
		// WireGuard is disabled; nothing to reconcile.
		return nil
	}
	if res.Spec.PublicKey == "" {
		return fmt.Errorf("public_key is required for reconciliation")
	}
	if res.Spec.AllowedIP == "" {
		return fmt.Errorf("allowed_ip is required for reconciliation")
	}
	return c.UpsertPeer(res.GetUID(), res.Spec.PublicKey, res.Spec.AllowedIP)
}

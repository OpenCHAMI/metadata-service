// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/openchami/fabrica/pkg/events"
	fabricaStorage "github.com/openchami/fabrica/pkg/storage"
	"github.com/openchami/metadata-service/internal/storage"
	"github.com/openchami/metadata-service/pkg/reconcilers"
	"github.com/openchami/metadata-service/pkg/wireguard"
)

var newReconciliationRuntimeFn = initializeReconciliationRuntime

// reconciliationRuntime wires Fabrica eventing to resource reconcilers. The
// event bus itself is owned by the generated feature loader; this runtime only
// manages its own subscriptions.
type reconciliationRuntime struct {
	eventBus                events.EventBus
	storageClient           *storage.StorageClient
	wireGuardPeerReconciler *reconcilers.WireGuardPeerReconciler
	wireGuardController     *wireguard.Controller
	subscriptionIDs         []events.SubscriptionID
	stopOnce                sync.Once
}

func initializeReconciliationRuntime(wgController *wireguard.Controller) (_ *reconciliationRuntime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("reconciliation runtime panic: %v\n%s", recovered, debug.Stack())
		}
	}()

	runtime := &reconciliationRuntime{wireGuardController: wgController}
	if err := runtime.initialize(); err != nil {
		_ = runtime.Stop()
		return nil, err
	}
	return runtime, nil
}

func (r *reconciliationRuntime) initialize() error {
	if err := r.initializeEventing(); err != nil {
		return err
	}

	r.storageClient = storage.NewStorageClient()
	r.wireGuardPeerReconciler = reconcilers.NewWireGuardPeerReconciler(r.storageClient, r.wireGuardController)

	if err := r.registerEventHooks(); err != nil {
		return err
	}

	if err := r.reconcileExistingResources(context.Background()); err != nil {
		return err
	}

	log.Printf("Reconciliation runtime initialized")
	return nil
}

func (r *reconciliationRuntime) initializeEventing() error {
	bus := events.GetGlobalEventBus()
	if bus == nil {
		return errors.New("event bus is not initialized; enable features.events in .fabrica.yaml and regenerate")
	}
	if !events.AreLifecycleEventsEnabled() {
		return errors.New("lifecycle events are disabled; WireGuardPeer reconciliation requires them")
	}
	r.eventBus = bus
	return nil
}

func (r *reconciliationRuntime) registerEventHooks() error {
	if r.eventBus == nil {
		return fmt.Errorf("event bus is not initialized")
	}

	subID, err := r.eventBus.Subscribe("**", func(ctx context.Context, event events.Event) error {
		return r.handleResourceEvent(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("subscribe to reconciliation events: %w", err)
	}
	r.subscriptionIDs = append(r.subscriptionIDs, subID)

	return nil
}

func (r *reconciliationRuntime) reconcileExistingResources(ctx context.Context) error {
	items, err := r.storageClient.List(ctx, "WireGuardPeer")
	if err != nil {
		return fmt.Errorf("list WireGuardPeer resources: %w", err)
	}

	for _, item := range items {
		if _, err := r.wireGuardPeerReconciler.Reconcile(r.withWireGuardController(ctx), item); err != nil {
			return fmt.Errorf("reconcile existing WireGuardPeer: %w", err)
		}
	}

	return nil
}

func (r *reconciliationRuntime) handleResourceEvent(ctx context.Context, event events.Event) error {
	if event.ResourceKind() != "WireGuardPeer" {
		return nil
	}

	uid := event.ResourceUID()
	if uid == "" {
		return nil
	}

	action := eventAction(event)
	ctx = r.withWireGuardController(ctx)

	switch action {
	case "delete", "deleted":
		return r.wireGuardPeerReconciler.Delete(ctx, uid)
	default:
		resource, err := r.storageClient.Get(ctx, "WireGuardPeer", uid)
		if err != nil {
			if errors.Is(err, fabricaStorage.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("load WireGuardPeer %s: %w", uid, err)
		}
		_, err = r.wireGuardPeerReconciler.Reconcile(ctx, resource)
		if err != nil {
			return fmt.Errorf("reconcile WireGuardPeer %s: %w", uid, err)
		}
		return nil
	}
}

func (r *reconciliationRuntime) withWireGuardController(ctx context.Context) context.Context {
	if r.wireGuardController == nil {
		return ctx
	}
	return context.WithValue(ctx, wireguard.ControllerContextKey, r.wireGuardController)
}

// Stop releases event subscriptions. The generated feature cleanup closes the
// event bus.
func (r *reconciliationRuntime) Stop() error {
	var stopErr error

	r.stopOnce.Do(func() {
		for _, subID := range r.subscriptionIDs {
			if err := r.eventBus.Unsubscribe(subID); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("unsubscribe %s: %w", subID, err))
			}
		}
		r.subscriptionIDs = nil
	})

	return stopErr
}

func eventAction(event events.Event) string {
	if rawAction, ok := event.Extensions()["action"]; ok {
		if action, ok := rawAction.(string); ok {
			return strings.ToLower(action)
		}
	}

	parts := strings.Split(strings.ToLower(event.Type()), ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

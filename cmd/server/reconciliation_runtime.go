// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenCHAMI/cloud-init/internal/storage"
	"github.com/OpenCHAMI/cloud-init/pkg/reconcilers"
	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
	"github.com/openchami/fabrica/pkg/events"
	"github.com/openchami/fabrica/pkg/reconcile"
)

func startReconciliationRuntime(ctx context.Context, wgController *wireguard.Controller) (func() error, error) {
	cfg := events.DefaultEventConfig()
	cfg.Enabled = true
	events.SetEventConfig(cfg)

	memoryBus := events.NewInMemoryEventBus(1000, 10)
	memoryBus.Start()
	events.SetGlobalEventBus(memoryBus)

	storageClient := storage.NewStorageClient()

	controller := reconcile.NewController(memoryBus, storage.Backend)
	if err := reconcilers.RegisterReconcilers(controller, storageClient, memoryBus); err != nil {
		_ = memoryBus.Close()
		return nil, fmt.Errorf("failed to register reconcilers: %w", err)
	}

	// Inject the WireGuard controller into the reconciler (nil-safe when WireGuard is disabled).
	reconcilers.SetWireGuardController(wgController)

	eventRegistry := reconcilers.NewEventHandlerRegistry(storageClient, memoryBus)
	if err := eventRegistry.RegisterEventHandlers(memoryBus); err != nil {
		_ = memoryBus.Close()
		return nil, fmt.Errorf("failed to register event handlers: %w", err)
	}

	if wgController != nil {
		_, err := memoryBus.Subscribe("**", func(_ context.Context, event events.Event) error {
			if !strings.EqualFold(event.ResourceKind(), "WireGuardPeer") {
				return nil
			}

			action, _ := event.Extensions()["action"].(string)
			if !strings.EqualFold(action, "deleted") {
				return nil
			}

			uid := strings.TrimSpace(event.ResourceUID())
			if uid == "" {
				return nil
			}

			return wgController.RemovePeerByID(uid)
		})
		if err != nil {
			_ = memoryBus.Close()
			return nil, fmt.Errorf("failed to register WireGuard delete event handler: %w", err)
		}
	}

	if err := controller.Start(ctx); err != nil {
		_ = memoryBus.Close()
		return nil, fmt.Errorf("failed to start reconciliation controller: %w", err)
	}

	stop := func() error {
		if err := controller.Stop(); err != nil {
			_ = memoryBus.Close()
			return err
		}
		return memoryBus.Close()
	}

	return stop, nil
}

// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"

	cloudinitv1 "github.com/OpenCHAMI/cloud-init/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/cloud-init/internal/authz"
	"github.com/OpenCHAMI/cloud-init/internal/storage"
	"github.com/OpenCHAMI/cloud-init/pkg/handlers"
	"github.com/OpenCHAMI/cloud-init/pkg/smdclient"
	"github.com/go-chi/chi/v5"
)

// StorageAdapter adapts the storage backend to the handlers.Store interface
type StorageAdapter struct{}

// NewStorageAdapter creates a new storage adapter
func NewStorageAdapter() *StorageAdapter {
	return &StorageAdapter{}
}

// GetClusterDefaults retrieves cluster defaults from storage
func (s *StorageAdapter) GetClusterDefaults() (*handlers.ClusterDefaults, error) {
	ctx := context.Background()

	// Get the first (and presumably only) ClusterDefaults resource
	resources, err := storage.LoadAllClusterDefaultss(ctx)
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, nil
	}

	// Get the first ClusterDefaults
	cd := resources[0]

	return &handlers.ClusterDefaults{
		BaseURL:          cd.Spec.BaseURL,
		CloudProvider:    cd.Spec.CloudProvider,
		Region:           cd.Spec.Region,
		AvailabilityZone: cd.Spec.AvailabilityZone,
		ClusterName:      cd.Spec.ClusterName,
		ShortName:        cd.Spec.ShortName,
		NidLength:        cd.Spec.NidLength,
		PublicKeys:       cd.Spec.PublicKeys,
	}, nil
}

// GetInstanceInfo retrieves instance-specific information from storage
func (s *StorageAdapter) GetInstanceInfo(id string) (*handlers.InstanceInfo, error) {
	ctx := context.Background()

	ii, err := storage.LoadInstanceInfo(ctx, id)
	if err != nil {
		return nil, err
	}

	return &handlers.InstanceInfo{
		InstanceID:       ii.Spec.InstanceID,
		LocalHostname:    ii.Spec.LocalHostname,
		Hostname:         ii.Spec.Hostname,
		CloudInitBaseURL: ii.Spec.CloudInitBaseURL,
		PublicKeys:       ii.Spec.PublicKeys,
	}, nil
}

// GetGroupData retrieves group data from storage
func (s *StorageAdapter) GetGroupData(name string) (*cloudinitv1.Group, error) {
	ctx := context.Background()

	g, err := storage.LoadGroup(ctx, name)
	if err != nil {
		return nil, err
	}

	return g, nil
}

// RegisterCloudInitRoutes registers the cloud-init metadata server endpoints
func RegisterCloudInitRoutes(r chi.Router, smd smdclient.SMDClient, store handlers.Store) {
	// Cloud-init metadata endpoints
	// These endpoints are consumed by cloud-init and should be callable without
	// OpenCHAMI authn/authz.
	r.Method(http.MethodGet, "/meta-data", authz.AnnotateRoute(http.MethodGet, "/meta-data", authz.Public(), handlers.MetaDataHandler(smd, store)))
	r.Method(http.MethodGet, "/user-data", authz.AnnotateRoute(http.MethodGet, "/user-data", authz.Public(), http.HandlerFunc(handlers.UserDataHandler)))
	r.Method(http.MethodGet, "/vendor-data", authz.AnnotateRoute(http.MethodGet, "/vendor-data", authz.Public(), handlers.VendorDataHandler(smd, store)))
	r.Method(http.MethodGet, "/network-config", authz.AnnotateRoute(http.MethodGet, "/network-config", authz.Public(), handlers.NetworkConfigHandler(smd, store)))
	r.Method(http.MethodGet, "/{group}.yaml", authz.AnnotateRoute(http.MethodGet, "/{group}.yaml", authz.Public(), handlers.GroupUserDataHandler(smd, store)))
}

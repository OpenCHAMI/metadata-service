// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"

	cloudinitv1 "github.com/OpenCHAMI/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/metadata-service/internal/storage"
	"github.com/OpenCHAMI/metadata-service/pkg/handlers"
	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/go-chi/chi/v5"
)

// StorageAdapter adapts the storage backend to the handlers.Store interface
type StorageAdapter struct{}

// NewStorageAdapter creates a new storage adapter
func NewStorageAdapter() *StorageAdapter {
	return &StorageAdapter{}
}

func latestClusterDefaults(resources []*cloudinitv1.ClusterDefaults) *cloudinitv1.ClusterDefaults {
	latest := resources[0]
	for _, resource := range resources[1:] {
		if resource.Metadata.UpdatedAt.After(latest.Metadata.UpdatedAt) {
			latest = resource
		}
	}
	return latest
}

func latestInstanceInfoByName(resources []*cloudinitv1.InstanceInfo, id string) *cloudinitv1.InstanceInfo {
	var latest *cloudinitv1.InstanceInfo
	for _, resource := range resources {
		if resource.Metadata.Name != id && resource.Spec.InstanceID != id {
			continue
		}
		if latest == nil || resource.Metadata.UpdatedAt.After(latest.Metadata.UpdatedAt) {
			latest = resource
		}
	}
	return latest
}

func latestGroupByName(resources []*cloudinitv1.Group, name string) *cloudinitv1.Group {
	var latest *cloudinitv1.Group
	for _, resource := range resources {
		if resource.Metadata.Name != name {
			continue
		}
		if latest == nil || resource.Metadata.UpdatedAt.After(latest.Metadata.UpdatedAt) {
			latest = resource
		}
	}
	return latest
}

// GetClusterDefaults retrieves cluster defaults from storage
func (s *StorageAdapter) GetClusterDefaults() (*handlers.ClusterDefaults, error) {
	ctx := context.Background()

	resources, err := storage.LoadAllClusterDefaultss(ctx)
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, nil
	}

	cd := latestClusterDefaults(resources)

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

	resources, err := storage.LoadAllInstanceInfos(ctx)
	if err != nil {
		return nil, err
	}

	ii := latestInstanceInfoByName(resources, id)
	if ii == nil {
		return nil, fmt.Errorf("no instance info for %s", id)
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

	resources, err := storage.LoadAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	g := latestGroupByName(resources, name)
	if g == nil {
		return nil, fmt.Errorf("no data for group %s", name)
	}

	return g, nil
}

// RegisterCloudInitRoutes registers the cloud-init metadata server endpoints
func RegisterCloudInitRoutes(r chi.Router, smd smdclient.SMDClient, store handlers.Store) {
	// Cloud-init metadata endpoints
	r.Get("/meta-data", handlers.MetaDataHandler(smd, store))
	r.Get("/user-data", handlers.UserDataHandler)
	r.Get("/vendor-data", handlers.VendorDataHandler(smd, store))
	r.Get("/network-config", handlers.NetworkConfigHandler(smd, store))
	r.Get("/{group}.yaml", handlers.GroupUserDataHandler(smd, store))
}

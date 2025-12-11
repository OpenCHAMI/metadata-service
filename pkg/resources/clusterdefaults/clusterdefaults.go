// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

// Package clusterdefaults provides the ClusterDefaults resource definition and management.
package clusterdefaults

import (
	"context"

	"github.com/openchami/fabrica/pkg/resource"
)

// ClusterDefaults represents a ClusterDefaults resource
type ClusterDefaults struct {
	resource.Resource
	Spec   ClusterDefaultsSpec   `json:"spec" validate:"required"`
	Status ClusterDefaultsStatus `json:"status,omitempty"`
}

// ClusterDefaultsSpec defines the desired state of ClusterDefaults
type ClusterDefaultsSpec struct { //nolint: revive
	Description      string   `json:"description,omitempty" validate:"max=200"`
	BaseURL          string   `json:"base_url" validate:"required,url"`
	CloudProvider    string   `json:"cloud_provider,omitempty"`
	Region           string   `json:"region,omitempty"`
	AvailabilityZone string   `json:"availability_zone,omitempty"`
	ClusterName      string   `json:"cluster_name" validate:"required"`
	ShortName        string   `json:"short_name,omitempty"`
	NidLength        int      `json:"nid_length,omitempty" validate:"gte=0,lte=10"`
	PublicKeys       []string `json:"public_keys,omitempty"`
}

// ClusterDefaultsStatus defines the observed state of ClusterDefaults
type ClusterDefaultsStatus struct { //nolint: revive
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
	Ready   bool   `json:"ready"`
	// Add your status fields here
}

// Validate implements custom validation logic for ClusterDefaults
func (r *ClusterDefaults) Validate(ctx context.Context) error { //nolint: revive
	// Add custom validation logic here
	// Example:
	// if r.Spec.Name == "forbidden" {
	//     return errors.New("name 'forbidden' is not allowed")
	// }

	return nil
}

// GetKind returns the kind of the resource
func (r *ClusterDefaults) GetKind() string {
	return "ClusterDefaults"
}

// GetName returns the name of the resource
func (r *ClusterDefaults) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource
func (r *ClusterDefaults) GetUID() string {
	return r.Metadata.UID
}

func init() {
	// Register resource type prefix for storage
	resource.RegisterResourcePrefix("ClusterDefaults", "clu")
}

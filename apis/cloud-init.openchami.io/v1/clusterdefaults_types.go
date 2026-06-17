// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package v1

import (
	"context"

	"github.com/openchami/fabrica/pkg/fabrica"
)

// ClusterDefaults represents a ClusterDefaults resource (hub/storage version).
type ClusterDefaults struct {
	APIVersion string                `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                `json:"kind" yaml:"kind"`
	Metadata   fabrica.Metadata      `json:"metadata" yaml:"metadata"`
	Spec       ClusterDefaultsSpec   `json:"spec" yaml:"spec"`
	Status     ClusterDefaultsStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// ClusterDefaultsSpec defines the desired state of ClusterDefaults.
type ClusterDefaultsSpec struct { //nolint: revive
	Description      string   `json:"description,omitempty" yaml:"description,omitempty" validate:"max=200"`
	BaseURL          string   `json:"base_url" yaml:"base_url" validate:"required,url"`
	CloudProvider    string   `json:"cloud_provider,omitempty" yaml:"cloud_provider,omitempty"`
	Region           string   `json:"region,omitempty" yaml:"region,omitempty"`
	AvailabilityZone string   `json:"availability_zone,omitempty" yaml:"availability_zone,omitempty"`
	ClusterName      string   `json:"cluster_name" yaml:"cluster_name" validate:"required"`
	ShortName        string   `json:"short_name,omitempty" yaml:"short_name,omitempty"`
	NidLength        int      `json:"nid_length,omitempty" yaml:"nid_length,omitempty" validate:"gte=0,lte=10"`
	PublicKeys       []string `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
}

// ClusterDefaultsStatus defines the observed state of ClusterDefaults.
type ClusterDefaultsStatus struct { //nolint: revive
	Phase   string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Ready   bool   `json:"ready" yaml:"ready"`
}

// IsHub marks this as the hub/storage version.
func (ClusterDefaults) IsHub() {}

// Validate implements custom validation logic for ClusterDefaults.
func (r *ClusterDefaults) Validate(ctx context.Context) error { //nolint: revive
	return nil
}

// GetKind returns the kind of the resource.
func (r *ClusterDefaults) GetKind() string {
	return "ClusterDefaults"
}

// GetName returns the name of the resource.
func (r *ClusterDefaults) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource.
func (r *ClusterDefaults) GetUID() string {
	return r.Metadata.UID
}

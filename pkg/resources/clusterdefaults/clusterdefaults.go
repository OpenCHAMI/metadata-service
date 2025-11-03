// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

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
type ClusterDefaultsSpec struct {
	Description string `json:"description,omitempty" validate:"max=200"`
	// Add your spec fields here
}

// ClusterDefaultsStatus defines the observed state of ClusterDefaults
type ClusterDefaultsStatus struct {
	Phase      string `json:"phase,omitempty"`
	Message    string `json:"message,omitempty"`
	Ready      bool   `json:"ready"`
	// Add your status fields here
}

// Validate implements custom validation logic for ClusterDefaults
func (r *ClusterDefaults) Validate(ctx context.Context) error {
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

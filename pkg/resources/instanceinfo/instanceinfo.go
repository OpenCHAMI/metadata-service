// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package instanceinfo

import (
	"context"
	"github.com/openchami/fabrica/pkg/resource"
)

// InstanceInfo represents a InstanceInfo resource
type InstanceInfo struct {
	resource.Resource
	Spec   InstanceInfoSpec   `json:"spec" validate:"required"`
	Status InstanceInfoStatus `json:"status,omitempty"`
}

// InstanceInfoSpec defines the desired state of InstanceInfo
type InstanceInfoSpec struct {
	Description string `json:"description,omitempty" validate:"max=200"`
	// Add your spec fields here
}

// InstanceInfoStatus defines the observed state of InstanceInfo
type InstanceInfoStatus struct {
	Phase      string `json:"phase,omitempty"`
	Message    string `json:"message,omitempty"`
	Ready      bool   `json:"ready"`
	// Add your status fields here
}

// Validate implements custom validation logic for InstanceInfo
func (r *InstanceInfo) Validate(ctx context.Context) error {
	// Add custom validation logic here
	// Example:
	// if r.Spec.Name == "forbidden" {
	//     return errors.New("name 'forbidden' is not allowed")
	// }

	return nil
}
// GetKind returns the kind of the resource
func (r *InstanceInfo) GetKind() string {
	return "InstanceInfo"
}
	
// GetName returns the name of the resource
func (r *InstanceInfo) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource
func (r *InstanceInfo) GetUID() string {
	return r.Metadata.UID
}

func init() {
	// Register resource type prefix for storage
	resource.RegisterResourcePrefix("InstanceInfo", "ins")
}

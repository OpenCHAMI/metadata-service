// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package v1

import (
	"context"

	"github.com/openchami/fabrica/pkg/fabrica"
)

// InstanceInfo represents an InstanceInfo resource (hub/storage version).
type InstanceInfo struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"`
	Metadata   fabrica.Metadata   `json:"metadata" yaml:"metadata"`
	Spec       InstanceInfoSpec   `json:"spec" yaml:"spec"`
	Status     InstanceInfoStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// InstanceInfoSpec defines the desired state of InstanceInfo.
type InstanceInfoSpec struct { //nolint: revive
	Description      string   `json:"description,omitempty" yaml:"description,omitempty" validate:"max=200"`
	InstanceID       string   `json:"instance_id" yaml:"instance_id" validate:"required"`
	LocalHostname    string   `json:"local_hostname,omitempty" yaml:"local_hostname,omitempty"`
	Hostname         string   `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	CloudInitBaseURL string   `json:"cloud_init_base_url,omitempty" yaml:"cloud_init_base_url,omitempty" validate:"omitempty,url"`
	PublicKeys       []string `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
	DefaultProfile   string   `json:"default_profile,omitempty" yaml:"default_profile,omitempty"`
}

// InstanceInfoStatus defines the observed state of InstanceInfo.
type InstanceInfoStatus struct { //nolint: revive
	Phase   string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Ready   bool   `json:"ready" yaml:"ready"`
}

// IsHub marks this as the hub/storage version.
func (InstanceInfo) IsHub() {}

// Validate implements custom validation logic for InstanceInfo.
func (r *InstanceInfo) Validate(ctx context.Context) error { //nolint: revive
	return nil
}

// GetKind returns the kind of the resource.
func (r *InstanceInfo) GetKind() string {
	return "InstanceInfo"
}

// GetName returns the name of the resource.
func (r *InstanceInfo) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource.
func (r *InstanceInfo) GetUID() string {
	return r.Metadata.UID
}

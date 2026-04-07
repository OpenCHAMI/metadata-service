// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package v1

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/openchami/fabrica/pkg/fabrica"
)

// Profile represents a Profile resource (hub/storage version).
type Profile struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   fabrica.Metadata `json:"metadata"`
	Spec       ProfileSpec      `json:"spec"`
	Status     ProfileStatus    `json:"status,omitempty"`
}

// ProfileSpec defines the desired state of Profile.
type ProfileSpec struct { //nolint: revive
	GroupRef         string            `json:"groupRef" validate:"required"`
	ParentProfile    string            `json:"parentProfile,omitempty"`
	Template         string            `json:"template,omitempty"`
	TemplateEncoding string            `json:"templateEncoding,omitempty"`
	MetaData         map[string]string `json:"metaData,omitempty"`
	ExpiresAt        string            `json:"expires_at,omitempty"`
	TTLSeconds       int               `json:"ttl_seconds,omitempty"`
}

// ProfileStatus defines the observed state of Profile.
type ProfileStatus struct { //nolint: revive
	Expired   bool   `json:"expired"`
	ExpiredAt string `json:"expired_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// IsHub marks this as the hub/storage version.
func (Profile) IsHub() {}

// GetKind returns the kind of the resource.
func (r *Profile) GetKind() string {
	return "Profile"
}

// GetName returns the name of the resource.
func (r *Profile) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource.
func (r *Profile) GetUID() string {
	return r.Metadata.UID
}

// Validate implements custom validation logic for Profile.
func (r *Profile) Validate(ctx context.Context) error { //nolint: revive
	if r.Spec.GroupRef == "" {
		return fmt.Errorf("groupRef is required")
	}

	if r.Spec.Template != "" {
		decoded, err := decodeProfileTemplateIfNeeded(r.Spec.Template, r.Spec.TemplateEncoding)
		if err != nil {
			return err
		}
		r.Spec.Template = decoded
		if r.Spec.TemplateEncoding != "" {
			r.Spec.TemplateEncoding = ""
		}
	}

	expiresAt := r.Spec.ExpiresAt
	if expiresAt == "" && r.Spec.TTLSeconds > 0 {
		if r.Status.ExpiresAt != "" {
			expiresAt = r.Status.ExpiresAt
		} else {
			expiresAt = time.Now().UTC().Add(time.Duration(r.Spec.TTLSeconds) * time.Second).Format(time.RFC3339)
		}
	}
	if expiresAt != "" {
		r.Status.ExpiresAt = expiresAt
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return fmt.Errorf("invalid expires_at: %w", err)
		}
		if time.Now().UTC().After(parsed) {
			r.Status.Expired = true
			r.Status.ExpiredAt = parsed.Format(time.RFC3339)
		} else {
			r.Status.Expired = false
			r.Status.ExpiredAt = ""
		}
	}

	return nil
}

func decodeProfileTemplateIfNeeded(template, encoding string) (string, error) {
	enc := strings.ToLower(strings.TrimSpace(encoding))
	switch enc {
	case "", "plain":
		return template, nil
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(template)
		if err != nil {
			return "", fmt.Errorf("template base64 decode error: %w", err)
		}
		return string(decoded), nil
	default:
		return "", fmt.Errorf("unsupported template encoding: %s", encoding)
	}
}

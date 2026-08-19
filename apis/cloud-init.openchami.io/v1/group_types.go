// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package v1

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openchami/fabrica/pkg/fabrica"
)

// Group represents a Group resource (hub/storage version).
type Group struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   fabrica.Metadata `json:"metadata" yaml:"metadata"`
	Spec       GroupSpec        `json:"spec" yaml:"spec"`
	Status     GroupStatus      `json:"status,omitempty" yaml:"status,omitempty"`
}

// GroupSpec defines the desired state of Group.
type GroupSpec struct { //nolint: revive
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Template    string            `json:"template,omitempty" yaml:"template,omitempty"`
	MetaData    map[string]string `json:"metaData,omitempty" yaml:"metaData,omitempty"`
	OSVersion   string            `json:"osVersion,omitempty" yaml:"osVersion,omitempty"`
}

// GroupStatus defines the observed state of Group.
type GroupStatus struct { //nolint: revive
	LastApplied            string                `json:"lastApplied,omitempty" yaml:"lastApplied,omitempty"`
	Valid                  bool                  `json:"valid" yaml:"valid"`
	ErrorMessage           string                `json:"errorMessage,omitempty" yaml:"errorMessage,omitempty"`
	ErrorDetails           string                `json:"errorDetails,omitempty" yaml:"errorDetails,omitempty"`
	TemplateHash           string                `json:"templateHash,omitempty" yaml:"templateHash,omitempty"`
	TemplateVersion        string                `json:"templateVersion,omitempty" yaml:"templateVersion,omitempty"`
	CurrentTemplateVersion string                `json:"currentTemplateVersion,omitempty" yaml:"currentTemplateVersion,omitempty"`
	Version                string                `json:"version,omitempty" yaml:"version,omitempty"`
	TemplateHistory        []TemplateVersionInfo `json:"templateHistory,omitempty" yaml:"templateHistory,omitempty"`
	RequiredVariables      []string              `json:"requiredVariables,omitempty" yaml:"requiredVariables,omitempty"`
}

// TemplateVersionInfo tracks template history.
type TemplateVersionInfo struct {
	Version      string `json:"version" yaml:"version"`
	Timestamp    string `json:"timestamp" yaml:"timestamp"`
	Valid        bool   `json:"valid" yaml:"valid"`
	ErrorMessage string `json:"errorMessage,omitempty" yaml:"errorMessage,omitempty"`
}

// IsHub marks this as the hub/storage version.
func (Group) IsHub() {}

// GetKind returns the kind of the resource.
func (r *Group) GetKind() string {
	return "Group"
}

// GetName returns the name of the resource.
func (r *Group) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource.
func (r *Group) GetUID() string {
	return r.Metadata.UID
}

// MergeMetadata combines default and group metadata.
func MergeMetadata(defaultMeta map[string]any, groupMeta map[string]string) map[string]interface{} {
	merged := make(map[string]interface{})
	for k, v := range defaultMeta {
		merged[k] = v
	}
	for k, v := range groupMeta {
		merged[k] = v
	}
	return merged
}

func hasTemplateVariableData(metadata map[string]interface{}, variable string) bool {
	current := any(metadata)
	for _, part := range strings.Split(variable, ".") {
		switch typed := current.(type) {
		case map[string]interface{}:
			next, ok := typed[part]
			if !ok {
				return false
			}
			current = next
		default:
			return false
		}
	}
	return true
}

// extractTemplateVariables finds {{var}} references.
func extractTemplateVariables(tmpl string) []string {
	re := regexp.MustCompile(`{{\s*([a-zA-Z0-9_\.]+)\s*}}`)
	matches := re.FindAllStringSubmatch(tmpl, -1)
	vars := make(map[string]struct{})
	for _, m := range matches {
		if len(m) > 1 {
			vars[m[1]] = struct{}{}
			if dot := regexp.MustCompile(`^([a-zA-Z0-9_]+)\.`); dot.MatchString(m[1]) {
				root := dot.FindStringSubmatch(m[1])[1]
				vars[root] = struct{}{}
			}
		}
	}
	loopRe := regexp.MustCompile(`{%\s*for\s+[a-zA-Z0-9_]+\s+in\s+([a-zA-Z0-9_\.]+)\s*%}`)
	loopMatches := loopRe.FindAllStringSubmatch(tmpl, -1)
	for _, m := range loopMatches {
		if len(m) > 1 {
			vars[m[1]] = struct{}{}
			if dot := regexp.MustCompile(`^([a-zA-Z0-9_]+)\.`); dot.MatchString(m[1]) {
				root := dot.FindStringSubmatch(m[1])[1]
				vars[root] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(vars))
	for v := range vars {
		out = append(out, v)
	}
	return out
}

// sampleMetadata returns example metadata for validation.
// Returns a cloud-init datasource-compliant structure wrapped in 'ds' key.
func sampleMetadata() map[string]any {
	vendordata := map[string]any{
		"version":             "1.0",
		"cloud_init_base_url": "http://cloud-init.local",
		"cluster_name":        "test-cluster",
		"nid":                 int64(1000),
		"role":                "compute",
		"mac":                 "00:11:22:33:44:55",
		"groups": map[string]any{
			"rack1": map[string]any{
				"description":      "Rack 1",
				"syslog_forwarder": "syslog.rack1.local",
			},
		},
		"interfaces": []map[string]any{{
			"name":        "eth0",
			"mac":         "00:11:22:33:44:55",
			"description": "Node Management Network",
			"enabled":     true,
			"redfishid":   "1",
			"ip":          "192.0.2.1",
			"network":     "HMN",
			"ip_addresses": []map[string]string{{
				"ip":      "192.0.2.1",
				"network": "HMN",
			}},
		}},
	}

	// Build the core data structure that templates will access via ds.*
	coreData := map[string]any{
		"cluster_name":      "test-cluster",
		"base_url":          "http://cloud-init.local",
		"cloud_name":        "OpenCHAMI",
		"cloud_provider":    "OpenCHAMI",
		"region":            "us-west-1",
		"availability_zone": "us-west-1a",
		"local_hostname":    "test-host",
		"hostname":          "test-host",
		"instance_type":     "compute",
		"instance_id":       "x1000c0s0b0n0",
		"nid":               1000,
		"role":              "compute",
		"mac":               "00:11:22:33:44:55",
		"ip":                "192.0.2.1",
		"local_ipv4":        "192.0.2.1",
		"domain":            "example.com",
		"os_version":        "ubuntu-22.04",
		"public_keys":       "ssh-rsa AAAAB3Nza...",
		"tags":              "role=compute,env=prod",
		"interfaces":        vendordata["interfaces"],
		"vendor_data":       vendordata,
		"meta_data": map[string]any{
			"instance-id":    "x1000c0s0b0n0",
			"local-hostname": "test-host",
			"hostname":       "test-host",
			"cluster-name":   "test-cluster",
			"instance_data": map[string]any{
				"v1": map[string]any{
					"cloud_name":        "OpenCHAMI",
					"availability_zone": "us-west-1a",
					"instance_id":       "x1000c0s0b0n0",
					"instance_type":     "compute",
					"local_hostname":    "test-host",
					"region":            "us-west-1",
					"hostname":          "test-host",
					"local_ipv4":        "192.0.2.1",
					"cloud_provider":    "OpenCHAMI",
					"public_keys":       []string{"ssh-rsa AAAAB3Nza..."},
					"vendor_data":       vendordata,
				},
			},
		},
	}

	// Wrap in 'ds' (datasource) key for cloud-init compliance
	// This enables template variables like: {{ ds.meta_data.instance_data.v1.public_keys }}
	return map[string]any{
		"ds": coreData,
	}
}

// Validate implements custom validation logic for Group.
func (r *Group) Validate(ctx context.Context) error { //nolint: revive

	vars := extractTemplateVariables(r.Spec.Template)
	r.Status.RequiredVariables = vars

	r.Status.Valid = true
	r.Status.ErrorMessage = ""
	r.trackTemplateVersion(true, "")
	return nil
}

// trackTemplateVersion adds a new entry to the template history.
func (r *Group) trackTemplateVersion(valid bool, errorMsg string) {
	version := generateTemplateVersion(r.Spec.Template)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	versionInfo := TemplateVersionInfo{
		Version:      version,
		Timestamp:    timestamp,
		Valid:        valid,
		ErrorMessage: errorMsg,
	}

	if r.Status.TemplateHistory == nil {
		r.Status.TemplateHistory = make([]TemplateVersionInfo, 0)
	}

	for i, existing := range r.Status.TemplateHistory {
		if existing.Version == version {
			r.Status.TemplateHistory[i] = versionInfo
			r.Status.CurrentTemplateVersion = version
			return
		}
	}

	r.Status.TemplateHistory = append(r.Status.TemplateHistory, versionInfo)
	r.Status.CurrentTemplateVersion = version

	if len(r.Status.TemplateHistory) > 10 {
		r.Status.TemplateHistory = r.Status.TemplateHistory[len(r.Status.TemplateHistory)-10:]
	}
}

// generateTemplateVersion creates a version string from template content.
func generateTemplateVersion(template string) string {
	hash := sha256.Sum256([]byte(template))
	return fmt.Sprintf("v-%x", hash[:4])
}

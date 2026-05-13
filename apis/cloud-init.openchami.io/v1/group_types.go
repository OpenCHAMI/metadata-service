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

	pongo2 "github.com/flosch/pongo2/v6"
	"github.com/openchami/fabrica/pkg/fabrica"
	"gopkg.in/yaml.v3"
)

// Group represents a Group resource (hub/storage version).
type Group struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   fabrica.Metadata `json:"metadata"`
	Spec       GroupSpec        `json:"spec"`
	Status     GroupStatus      `json:"status,omitempty"`
}

// GroupSpec defines the desired state of Group.
type GroupSpec struct { //nolint: revive
	Description string            `json:"description,omitempty"`
	Template    string            `json:"template" validate:"required"`
	MetaData    map[string]string `json:"metaData,omitempty"`
	OSVersion   string            `json:"osVersion,omitempty"`
}

// GroupStatus defines the observed state of Group.
type GroupStatus struct { //nolint: revive
	LastApplied            string                `json:"lastApplied,omitempty"`
	Valid                  bool                  `json:"valid"`
	ErrorMessage           string                `json:"errorMessage,omitempty"`
	ErrorDetails           string                `json:"errorDetails,omitempty"`
	TemplateHash           string                `json:"templateHash,omitempty"`
	TemplateVersion        string                `json:"templateVersion,omitempty"`
	CurrentTemplateVersion string                `json:"currentTemplateVersion,omitempty"`
	Version                string                `json:"version,omitempty"`
	TemplateHistory        []TemplateVersionInfo `json:"templateHistory,omitempty"`
	RequiredVariables      []string              `json:"requiredVariables,omitempty"`
}

// TemplateVersionInfo tracks template history.
type TemplateVersionInfo struct {
	Version      string `json:"version"`
	Timestamp    string `json:"timestamp"`
	Valid        bool   `json:"valid"`
	ErrorMessage string `json:"errorMessage,omitempty"`
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

// RenderTemplate renders a Jinja2-compatible template using pongo2.
func RenderTemplate(templateStr string, metadata map[string]interface{}) (string, error) {
	templateSet := pongo2.NewSet("group-template", pongo2.MustNewLocalFileSystemLoader(""))
	return templateSet.RenderTemplateString(templateStr, metadata)
}

// validateYAML checks if a string is valid YAML.
func validateYAML(s string) error {
	var out interface{}
	return yaml.Unmarshal([]byte(s), &out)
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
func sampleMetadata() map[string]any {
	return map[string]any{
		"cluster_name":      "test-cluster",
		"base_url":          "http://cloud-init.local",
		"cloud_provider":    "OpenCHAMI",
		"region":            "us-west-1",
		"availability_zone": "us-west-1a",
		"hostname":          "test-host",
		"instance_id":       "x1000c0s0b0n0",
		"nid":               1000,
		"role":              "compute",
		"mac":               "00:11:22:33:44:55",
		"ip":                "192.0.2.1",
		"domain":            "example.com",
		"os_version":        "ubuntu-22.04",
		"ssh_keys":          "ssh-rsa AAAAB3Nza...",
		"tags":              "role=compute,env=prod",
		"vendor_data": map[string]any{
			"version":             "1.0",
			"cloud_init_base_url": "http://cloud-init.local",
			"cluster_name":        "test-cluster",
			"nid":                 1000,
			"role":                "compute",
			"mac":                 "00:11:22:33:44:55",
			"groups": map[string]any{
				"rack1": map[string]any{
					"syslog_forwarder": "syslog.rack1.local",
				},
			},
		},
		"meta_data": map[string]any{
			"instance-id":    "x1000c0s0b0n0",
			"local-hostname": "test-host",
			"hostname":       "test-host",
			"cluster-name":   "test-cluster",
			"instance_data": map[string]any{
				"v1": map[string]any{
					"vendor_data": map[string]any{
						"groups": map[string]any{
							"rack1": map[string]any{
								"syslog_forwarder": "syslog.rack1.local",
							},
						},
					},
				},
			},
		},
	}
}

// Validate implements custom validation logic for Group.
func (r *Group) Validate(ctx context.Context) error { //nolint: revive
	if r.Spec.Template == "" {
		r.Status.Valid = false
		r.Status.ErrorMessage = "template is required"
		r.trackTemplateVersion(false, "template is required")
		return fmt.Errorf("template is required")
	}

	vars := extractTemplateVariables(r.Spec.Template)
	r.Status.RequiredVariables = vars
	merged := MergeMetadata(sampleMetadata(), r.Spec.MetaData)
	missing := []string{}
	for _, v := range vars {
		if !hasTemplateVariableData(merged, v) {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		r.Status.Valid = false
		r.Status.ErrorMessage = "missing required variables: " + fmt.Sprintf("%v", missing)
		r.trackTemplateVersion(false, r.Status.ErrorMessage)
		return fmt.Errorf("%s", r.Status.ErrorMessage)
	}

	rendered, err := RenderTemplate(r.Spec.Template, merged)
	if err != nil {
		r.Status.Valid = false
		r.Status.ErrorMessage = "template render error: " + err.Error()
		r.trackTemplateVersion(false, r.Status.ErrorMessage)
		return fmt.Errorf("%s", r.Status.ErrorMessage)
	}

	if err := validateYAML(rendered); err != nil {
		r.Status.Valid = false
		r.Status.ErrorMessage = "template YAML invalid after rendering: " + err.Error()
		r.trackTemplateVersion(false, r.Status.ErrorMessage)
		return fmt.Errorf("%s", r.Status.ErrorMessage)
	}

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

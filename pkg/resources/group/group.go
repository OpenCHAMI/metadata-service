// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package group

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"time"

	pongo2 "github.com/flosch/pongo2/v6"
	"github.com/openchami/fabrica/pkg/resource"
	"gopkg.in/yaml.v3"
)

// Group represents a Group resource
type Group struct {
	resource.Resource
	Spec   GroupSpec   `json:"spec" validate:"required"`
	Status GroupStatus `json:"status,omitempty"`
}

// GroupSpec defines the desired state of Group
type GroupSpec struct {
	Description string            `json:"description,omitempty"`
	Template    string            `json:"template" validate:"required"`
	MetaData    map[string]string `json:"metaData,omitempty"`
	OSVersion   string            `json:"osVersion,omitempty"`
	// ...other fields
}

// GroupStatus defines the observed state of Group
type GroupStatus struct {
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
	// ...other status/history fields

}

// MergeMetadata combines default and group metadata
func MergeMetadata(defaultMeta, groupMeta map[string]string) map[string]interface{} {
	merged := make(map[string]interface{})
	for k, v := range defaultMeta {
		merged[k] = v
	}
	for k, v := range groupMeta {
		merged[k] = v
	}
	return merged
}

// RenderTemplate renders a Jinja2-compatible template using pongo2
func RenderTemplate(templateStr string, metadata map[string]interface{}) (string, error) {
	// import "github.com/flosch/pongo2/v6"
	tpl, err := pongo2.FromString(templateStr)
	if err != nil {
		return "", err
	}
	return tpl.Execute(metadata)
}

// validateYAML checks if a string is valid YAML
func validateYAML(s string) error {
	// import "gopkg.in/yaml.v3"
	var out interface{}
	return yaml.Unmarshal([]byte(s), &out)
}

// extractTemplateVariables finds {{var}} references
func extractTemplateVariables(tmpl string) []string {
	// import "regexp"
	// Detect simple and dotted variables: {{foo}}, {{user.name}}, {{group.meta.key}}
	re := regexp.MustCompile(`{{\s*([a-zA-Z0-9_\.]+)\s*}}`)
	matches := re.FindAllStringSubmatch(tmpl, -1)
	vars := make(map[string]struct{})
	for _, m := range matches {
		if len(m) > 1 {
			vars[m[1]] = struct{}{}
			// Also add the root variable (before dot) for operator visibility
			if dot := regexp.MustCompile(`^([a-zA-Z0-9_]+)\.`); dot.MatchString(m[1]) {
				root := dot.FindStringSubmatch(m[1])[1]
				vars[root] = struct{}{}
			}
		}
	}
	// Also detect array variables in Jinja2 for-loops: {% for item in array %} and dotted: {% for key in user.ssh_keys %}
	loopRe := regexp.MustCompile(`{%\s*for\s+[a-zA-Z0-9_]+\s+in\s+([a-zA-Z0-9_\.]+)\s*%}`)
	loopMatches := loopRe.FindAllStringSubmatch(tmpl, -1)
	for _, m := range loopMatches {
		if len(m) > 1 {
			vars[m[1]] = struct{}{}
			// Also add the root variable for dotted arrays
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

// sampleMetadata returns example metadata for validation
// This includes variables that will be provided at runtime from:
// - ClusterDefaults: cluster_name, base_url, cloud_provider, region, availability_zone
// - SMD Component: instance_id, hostname, nid, role, mac, ip
func sampleMetadata() map[string]string {
	return map[string]string{
		// Runtime variables from ClusterDefaults
		"cluster_name":      "test-cluster",
		"base_url":          "http://cloud-init.local",
		"cloud_provider":    "OpenCHAMI",
		"region":            "us-west-1",
		"availability_zone": "us-west-1a",

		// Runtime variables from SMD Component
		"hostname":    "test-host",
		"instance_id": "x1000c0s0b0n0",
		"nid":         "1000",
		"role":        "compute",
		"mac":         "00:11:22:33:44:55",
		"ip":          "192.0.2.1",

		// Additional common variables
		"domain":     "example.com",
		"os_version": "ubuntu-22.04",
		"ssh_keys":   "ssh-rsa AAAAB3Nza...",
		"tags":       "role=compute,env=prod",
	}
}

// TemplateVersionInfo tracks template history
type TemplateVersionInfo struct {
	Version      string `json:"version"`
	Timestamp    string `json:"timestamp"`
	Valid        bool   `json:"valid"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// Validate implements custom validation logic for Group
func (r *Group) Validate(ctx context.Context) error {
	// Shared template validation logic
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
		if _, ok := merged[v]; !ok {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		r.Status.Valid = false
		r.Status.ErrorMessage = "missing required variables: " + fmt.Sprintf("%v", missing)
		r.trackTemplateVersion(false, r.Status.ErrorMessage)
		return fmt.Errorf(r.Status.ErrorMessage)
	}

	rendered, err := RenderTemplate(r.Spec.Template, merged)
	if err != nil {
		r.Status.Valid = false
		r.Status.ErrorMessage = "template render error: " + err.Error()
		r.trackTemplateVersion(false, r.Status.ErrorMessage)
		return fmt.Errorf(r.Status.ErrorMessage)
	}

	if err := validateYAML(rendered); err != nil {
		r.Status.Valid = false
		r.Status.ErrorMessage = "template YAML invalid after rendering: " + err.Error()
		r.trackTemplateVersion(false, r.Status.ErrorMessage)
		return fmt.Errorf(r.Status.ErrorMessage)
	}

	r.Status.Valid = true
	r.Status.ErrorMessage = ""
	r.trackTemplateVersion(true, "")
	// Debug: Print template history length
	fmt.Printf("DEBUG: Template history length after tracking: %d, current version: %s\n", len(r.Status.TemplateHistory), r.Status.CurrentTemplateVersion)
	return nil
}

// trackTemplateVersion adds a new entry to the template history
func (r *Group) trackTemplateVersion(valid bool, errorMsg string) {
	// Generate version string based on template hash
	version := generateTemplateVersion(r.Spec.Template)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Create new version entry
	versionInfo := TemplateVersionInfo{
		Version:      version,
		Timestamp:    timestamp,
		Valid:        valid,
		ErrorMessage: errorMsg,
	}

	// Initialize history if needed
	if r.Status.TemplateHistory == nil {
		r.Status.TemplateHistory = make([]TemplateVersionInfo, 0)
	}

	// Check if this version already exists (avoid duplicates on re-validation)
	for i, existing := range r.Status.TemplateHistory {
		if existing.Version == version {
			// Update existing entry
			r.Status.TemplateHistory[i] = versionInfo
			r.Status.CurrentTemplateVersion = version
			return
		}
	}

	// Add new version to history
	r.Status.TemplateHistory = append(r.Status.TemplateHistory, versionInfo)
	r.Status.CurrentTemplateVersion = version

	// Keep only last 10 versions to avoid unbounded growth
	if len(r.Status.TemplateHistory) > 10 {
		r.Status.TemplateHistory = r.Status.TemplateHistory[len(r.Status.TemplateHistory)-10:]
	}
}

// generateTemplateVersion creates a version string from template content
func generateTemplateVersion(template string) string {
	// Use first 8 chars of SHA256 hash as version
	hash := sha256.Sum256([]byte(template))
	return fmt.Sprintf("v-%x", hash[:4])
}

// GetKind returns the kind of the resource
func (r *Group) GetKind() string {
	return "Group"
}

// GetName returns the name of the resource
func (r *Group) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource
func (r *Group) GetUID() string {
	return r.Metadata.UID
}

func init() {
	// Register resource type prefix for storage
	resource.RegisterResourcePrefix("Group", "gro")
}

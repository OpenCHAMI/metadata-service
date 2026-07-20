// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package v1

import (
	"context"
	"strings"
	"testing"
)

func TestGroupValidateSuccess(t *testing.T) {
	t.Parallel()

	group := &Group{
		Spec: GroupSpec{
			Template: `#cloud-config
hostname: {{ ds.hostname }}
role: "{{ ds.role }}"
syslog: "{{ ds.vendor_data.groups.rack1.syslog_forwarder }}"
note: "{{ custom_message }}"
`,
			MetaData: map[string]string{
				"custom_message": "hello from test",
			},
		},
	}

	if err := group.Validate(context.Background()); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}

	if !group.Status.Valid {
		t.Fatalf("expected group to be valid, got status: %+v", group.Status)
	}

	if group.Status.ErrorMessage != "" {
		t.Fatalf("expected empty error message, got %q", group.Status.ErrorMessage)
	}

	if group.Status.CurrentTemplateVersion == "" {
		t.Fatal("expected current template version to be set")
	}

	if len(group.Status.TemplateHistory) != 1 {
		t.Fatalf("expected 1 template history entry, got %d", len(group.Status.TemplateHistory))
	}

	if group.Status.TemplateHistory[0].Version != group.Status.CurrentTemplateVersion {
		t.Fatalf("expected current version %q to match history entry %q", group.Status.CurrentTemplateVersion, group.Status.TemplateHistory[0].Version)
	}

	required := make(map[string]bool, len(group.Status.RequiredVariables))
	for _, variable := range group.Status.RequiredVariables {
		required[variable] = true
	}

	for _, expected := range []string{"ds", "ds.hostname", "ds.role", "ds.vendor_data.groups.rack1.syslog_forwarder", "custom_message"} {
		if !required[expected] {
			t.Fatalf("expected required variables to contain %q, got %v", expected, group.Status.RequiredVariables)
		}
	}
}

func TestGroupValidateMissingRequiredVariable(t *testing.T) {
	t.Parallel()

	group := &Group{
		Spec: GroupSpec{
			Template: `#cloud-config
hostname: {{ missing_value }}
`,
		},
	}

	err := group.Validate(context.Background())
	if err == nil {
		t.Fatal("expected Validate() to fail for missing variables")
	}

	if !strings.Contains(err.Error(), "missing required variables") {
		t.Fatalf("expected missing variable error, got %v", err)
	}

	if group.Status.Valid {
		t.Fatalf("expected invalid group status, got %+v", group.Status)
	}

	if !strings.Contains(group.Status.ErrorMessage, "missing_value") {
		t.Fatalf("expected missing variable in status error, got %q", group.Status.ErrorMessage)
	}

	if len(group.Status.TemplateHistory) != 1 {
		t.Fatalf("expected 1 template history entry, got %d", len(group.Status.TemplateHistory))
	}

	if group.Status.TemplateHistory[0].Valid {
		t.Fatalf("expected invalid template history entry, got %+v", group.Status.TemplateHistory[0])
	}
}

func TestGroupValidateDatasourcePaths(t *testing.T) {
	t.Parallel()

	group := &Group{
		Spec: GroupSpec{
			Template: `#cloud-config
hostname: {{ ds.hostname }}
merge_how:
- name: list
  settings: [append]
- name: dict
  settings: [no_replace, recurse_list]
users:
  - name: root
    ssh_authorized_keys: {{ ds.meta_data.instance_data.v1.public_keys }}
disable_root: false
`,
		},
	}

	if err := group.Validate(context.Background()); err != nil {
		t.Fatalf("Validate() returned error for ds.* paths: %v", err)
	}

	if !group.Status.Valid {
		t.Fatalf("expected group with ds.* paths to be valid, got status: %+v", group.Status)
	}

	if group.Status.ErrorMessage != "" {
		t.Fatalf("expected empty error message, got %q", group.Status.ErrorMessage)
	}

	required := make(map[string]bool, len(group.Status.RequiredVariables))
	for _, variable := range group.Status.RequiredVariables {
		required[variable] = true
	}

	for _, expected := range []string{"ds", "ds.hostname", "ds.meta_data.instance_data.v1.public_keys"} {
		if !required[expected] {
			t.Fatalf("expected required variables to contain %q, got %v", expected, group.Status.RequiredVariables)
		}
	}
}

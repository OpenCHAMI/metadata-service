package authz

import "testing"

func TestObjectConstants(t *testing.T) {
	if ObjectNodeMetadata != "node-metadata" {
		t.Fatalf("ObjectNodeMetadata = %q, want %q", ObjectNodeMetadata, "node-metadata")
	}
	if ObjectGroupMetadata != "group-metadata" {
		t.Fatalf("ObjectGroupMetadata = %q, want %q", ObjectGroupMetadata, "group-metadata")
	}
}

func TestActionConstants(t *testing.T) {
	if ActionRead != "read" {
		t.Fatalf("ActionRead = %q, want %q", ActionRead, "read")
	}
	if ActionWrite != "write" {
		t.Fatalf("ActionWrite = %q, want %q", ActionWrite, "write")
	}
	if ActionDelete != "delete" {
		t.Fatalf("ActionDelete = %q, want %q", ActionDelete, "delete")
	}
}

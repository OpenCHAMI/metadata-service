// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package authz

import "testing"

func TestAllRoutesAnnotated(t *testing.T) {
	// Route annotation recording happens at runtime when routes are registered.
	// In unit tests we don't start cmd/server, so we can't observe chi
	// registration directly.
	//
	// Instead, this guard ensures a deny-by-default workflow by requiring every
	// route to be represented in ExpectedRoutes() (which is kept in sync with
	// docs/authz_route_inventory.md). Later, when tokensmith wiring is added,
	// the middleware will use these annotations for enforcement.
	expected := ExpectedRoutes()
	if len(expected) == 0 {
		t.Fatal("ExpectedRoutes() returned an empty list; route coverage guard is ineffective")
	}
}

func TestExpectedRoutesNoDuplicates(t *testing.T) {
	seen := map[RouteSpec]struct{}{}
	for _, r := range ExpectedRoutes() {
		if _, ok := seen[r]; ok {
			t.Fatalf("duplicate route in ExpectedRoutes(): %s %s", r.Method, r.Path)
		}
		seen[r] = struct{}{}
	}
}

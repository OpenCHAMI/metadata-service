// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package authz

import "net/http"

// Route annotators used when registering handlers.
//
// These wrappers are intentionally minimal and are designed to align with the
// tokensmith authz contract:
//   - Public: route does not require authorization (and may skip authn)
//   - Require: route requires authorization against (obj, act)
//
// Step 2 wires these wrappers only; middleware implementation will be integrated
// in a later step.

// Annotator wraps an http.Handler with metadata for authn/authz middleware.
//
// Today this is a no-op to keep behavior unchanged until the middleware is
// introduced. It exists so route registration can be updated incrementally.
//
// NOTE: In a follow-up step, these will call into tokensmith's Public/Require
// wrappers directly.
type Annotator func(http.Handler) http.Handler

// Public marks a route as public.
func Public() Annotator {
	return func(next http.Handler) http.Handler { return next }
}

// Require marks a route as requiring (obj, act) authorization.
func Require(obj, act string) Annotator {
	_ = obj
	_ = act
	return func(next http.Handler) http.Handler { return next }
}

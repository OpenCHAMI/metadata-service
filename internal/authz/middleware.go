// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

package authz

import (
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	// EnvAuthzMode controls whether authorization is enabled.
	//
	// Values:
	//  - "off" (default): authorization disabled; middleware is a no-op.
	//  - "enforce": authorization enabled (actual checks added in a later step).
	EnvAuthzMode = "AUTHZ_MODE"

	// EnvAuthzPolicyDir optionally points at a directory containing Casbin policy/model files.
	// It is currently only used for startup diagnostics in this service.
	EnvAuthzPolicyDir = "AUTHZ_POLICY_DIR"
)

// WrapMiddleware returns a chi-compatible middleware.
//
// Step 3 behavior: default OFF and no enforcement. This is intentional to
// preserve existing behavior while wiring up consistent startup diagnostics
// and middleware ordering.
func WrapMiddleware() func(http.Handler) http.Handler {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(EnvAuthzMode)))
	if mode == "" {
		mode = "off"
	}
	policyDir := strings.TrimSpace(os.Getenv(EnvAuthzPolicyDir))

	log.Printf("authz: mode=%s", mode)
	if policyDir == "" {
		log.Printf("authz: policy_dir not set")
	} else {
		log.Printf("authz: policy_dir=%s", policyDir)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No-op for now; enforcement is added in a later step.
			next.ServeHTTP(w, r)
		})
	}
}

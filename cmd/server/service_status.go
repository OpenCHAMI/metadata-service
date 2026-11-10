// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import "github.com/openchami/metadata-service/pkg/smdclient"

// Back the Fabrica-generated public /service/status endpoint with the same SMD
// readiness signal used by /health, instead of the generated always-"ok" default.
func init() {
	serviceStatusProvider = smdServiceStatus
}

// smdServiceStatus maps SMD health onto the standardized service status
// vocabulary: "initializing" until the first SMD sync completes, "down" for any
// other unhealthy condition (e.g. TokenSmith auth failure), "ok" otherwise.
func smdServiceStatus() ServiceStatus {
	if currentSMDHealth == nil {
		return ServiceStatusOK
	}
	healthy, reason := currentSMDHealth.InitialSyncStatus()
	switch {
	case healthy:
		return ServiceStatusOK
	case reason == smdclient.InitialRefreshPendingReason:
		return ServiceStatusInitializing
	default:
		return ServiceStatusDown
	}
}

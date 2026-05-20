// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import "fmt"

// ResolveComponentID returns the component ID for a request IP, preferring a
// WireGuard reverse lookup before falling back to a direct IP lookup.
func ResolveComponentID(client SMDClient, ip string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("nil SMD client")
	}

	if resolver, ok := client.(ComponentResolver); ok {
		if id, err := resolver.ResolveComponentID(ip); err == nil && id != "" {
			return id, nil
		}
	}

	if id, err := client.IDfromWGIP(ip); err == nil && id != "" {
		return id, nil
	}

	if id, err := client.IDfromIP(ip); err == nil && id != "" {
		return id, nil
	}

	return "", fmt.Errorf("no component found for IP %s", ip)
}

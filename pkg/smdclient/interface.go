// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

// Component represents a hardware component in SMD
type Component struct {
	ID   string
	NID  int64
	Role string
	MAC  string
	IP   string
}

// SMDClient defines the interface for interacting with the State Management Database
type SMDClient interface {
	// IDfromIP returns the component ID for a given IP address
	IDfromIP(ip string) (string, error)

	// IPfromID returns the IP address for a given component ID
	IPfromID(id string) (string, error)

	// MACfromID returns the MAC address for a given component ID
	MACfromID(id string) (string, error)

	// ComponentInformation returns detailed information about a component
	ComponentInformation(id string) (*Component, error)

	// GroupMembership returns the list of groups a component belongs to
	GroupMembership(id string) ([]string, error)

	// AddWGIP records the allocated WireGuard IP for a component
	AddWGIP(id, wgip string) error

	// WGIPfromID returns the stored WireGuard IP for a component
	WGIPfromID(id string) (string, error)
}

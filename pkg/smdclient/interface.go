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

// EthernetNIC represents network interface information from RedfishSystemInfo
type EthernetNIC struct {
	RedfishID           string
	Description         string
	MACAddress          string
	PermanentMACAddress string
	InterfaceEnabled    bool
}

// IPMapping represents an IP address and network pair
type IPMapping struct {
	IPAddress string
	Network   string
}

// EthernetInterface represents a network interface with IP mappings from SMD
type EthernetInterface struct {
	ID          string
	Description string
	MACAddress  string
	IPAddresses []IPMapping
	ComponentID string
	Type        string
}

// SMDClient defines the interface for interacting with the State Management Database
type SMDClient interface {
	// IDfromIP returns the component ID for a given IP address
	IDfromIP(ip string) (string, error)

	// IDfromWGIP returns the component ID for a given WireGuard IP address
	IDfromWGIP(wgip string) (string, error)

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

	// EthernetNICInfo returns the list of network interfaces from RedfishSystemInfo
	EthernetNICInfo(id string) ([]EthernetNIC, error)

	// EthernetInterfaces returns the list of EthernetInterface entries for a component
	// with IP address and network mappings
	EthernetInterfaces(id string) ([]EthernetInterface, error)
}

// ComponentLister is implemented by SMD clients that can list all components.
// It is used by the integration sync worker to pre-populate cache state.
type ComponentLister interface {
	// ListComponents returns all known SMD components.
	ListComponents() ([]*Component, error)
}

// ComponentResolver is an optional interface for clients that provide an
// optimized ResolveComponentID implementation.
type ComponentResolver interface {
	ResolveComponentID(ip string) (string, error)
}

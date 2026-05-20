// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"fmt"
	"strings"
	"sync"
)

// MockSMDClient is a mock implementation of SMDClient for testing
type MockSMDClient struct {
	mu             sync.RWMutex
	components     map[string]*Component
	ipToID         map[string]string
	groups         map[string][]string // component ID -> group names
	wgip           map[string]string   // component ID -> WG IP
	ethernetNICs   map[string][]EthernetNIC
	ethernetIfaces map[string][]EthernetInterface
}

// NewMockSMDClient creates a new mock SMD client
func NewMockSMDClient() *MockSMDClient {
	return &MockSMDClient{
		components:     make(map[string]*Component),
		ipToID:         make(map[string]string),
		groups:         make(map[string][]string),
		wgip:           make(map[string]string),
		ethernetNICs:   make(map[string][]EthernetNIC),
		ethernetIfaces: make(map[string][]EthernetInterface),
	}
}

// AddComponent adds a component to the mock client
func (m *MockSMDClient) AddComponent(component *Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components[component.ID] = component
	if component.IP != "" {
		m.ipToID[component.IP] = component.ID
	}
}

// ListComponents returns all components known by the mock.
func (m *MockSMDClient) ListComponents() ([]*Component, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Component, 0, len(m.components))
	for _, component := range m.components {
		if component == nil {
			continue
		}
		copy := *component
		result = append(result, &copy)
	}
	return result, nil
}

// AddGroupMembership adds a component to a group
func (m *MockSMDClient) AddGroupMembership(componentID string, groups []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[componentID] = groups
}

// AddEthernetNICInfo adds network interface information for a component
func (m *MockSMDClient) AddEthernetNICInfo(componentID string, nics []EthernetNIC) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ethernetNICs[componentID] = nics
}

// AddEthernetInterfaces adds EthernetInterface entries for a component
func (m *MockSMDClient) AddEthernetInterfaces(componentID string, ifaces []EthernetInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ethernetIfaces[componentID] = ifaces
}

// IDfromIP returns the component ID for a given IP address
func (m *MockSMDClient) IDfromIP(ip string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id, ok := m.ipToID[ip]; ok {
		return id, nil
	}
	return "", fmt.Errorf("no component found for IP %s", ip)
}

// IDfromWGIP returns the component ID for a given WireGuard IP address
func (m *MockSMDClient) IDfromWGIP(wgip string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, storedWGIP := range m.wgip {
		if storedWGIP == wgip {
			return id, nil
		}
	}
	return "", fmt.Errorf("no component found for WireGuard IP %s", wgip)
}

// IPfromID returns the IP address for a given component ID
func (m *MockSMDClient) IPfromID(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ifaces, ok := m.ethernetIfaces[id]; ok {
		if ip := pickHMNIP(ifaces); ip != "" {
			return ip, nil
		}
	}
	if component, ok := m.components[id]; ok {
		return component.IP, nil
	}
	return "", fmt.Errorf("no component found for ID %s", id)
}

// MACfromID returns the MAC address for a given component ID
func (m *MockSMDClient) MACfromID(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ifaces, ok := m.ethernetIfaces[id]; ok {
		if mac := pickHMNMAC(ifaces); mac != "" {
			return mac, nil
		}
	}
	if component, ok := m.components[id]; ok {
		return component.MAC, nil
	}
	return "", fmt.Errorf("no component found for ID %s", id)
}

func pickHMNIP(ifaces []EthernetInterface) string {
	for _, iface := range ifaces {
		for _, ip := range iface.IPAddresses {
			if strings.EqualFold(ip.Network, "HMN") {
				return ip.IPAddress
			}
		}
	}
	for _, iface := range ifaces {
		for _, ip := range iface.IPAddresses {
			if ip.IPAddress != "" {
				return ip.IPAddress
			}
		}
	}
	return ""
}

func pickHMNMAC(ifaces []EthernetInterface) string {
	for _, iface := range ifaces {
		for _, ip := range iface.IPAddresses {
			if strings.EqualFold(ip.Network, "HMN") {
				return iface.MACAddress
			}
		}
	}
	for _, iface := range ifaces {
		if iface.MACAddress != "" {
			return iface.MACAddress
		}
	}
	return ""
}

// ComponentInformation returns detailed information about a component
func (m *MockSMDClient) ComponentInformation(id string) (*Component, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if component, ok := m.components[id]; ok {
		// Return a copy to prevent modification
		componentCopy := *component
		return &componentCopy, nil
	}
	return nil, fmt.Errorf("no component found for ID %s", id)
}

// GroupMembership returns the list of groups a component belongs to
func (m *MockSMDClient) GroupMembership(id string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if groups, ok := m.groups[id]; ok {
		// Return a copy to prevent modification
		result := make([]string, len(groups))
		copy(result, groups)
		return result, nil
	}
	return []string{}, nil
}

// AddWGIP records the allocated WireGuard IP for a component
func (m *MockSMDClient) AddWGIP(id, wgip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wgip[id] = wgip
	return nil
}

// WGIPfromID returns the stored WireGuard IP for a component
func (m *MockSMDClient) WGIPfromID(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ip, ok := m.wgip[id]; ok {
		return ip, nil
	}
	return "", fmt.Errorf("no WGIP found for ID %s", id)
}

// EthernetNICInfo returns the list of network interfaces from RedfishSystemInfo
func (m *MockSMDClient) EthernetNICInfo(id string) ([]EthernetNIC, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if nics, ok := m.ethernetNICs[id]; ok {
		// Return a copy to prevent modification
		result := make([]EthernetNIC, len(nics))
		copy(result, nics)
		return result, nil
	}
	return []EthernetNIC{}, nil
}

// EthernetInterfaces returns the list of EthernetInterface entries for a component
func (m *MockSMDClient) EthernetInterfaces(id string) ([]EthernetInterface, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ifaces, ok := m.ethernetIfaces[id]; ok {
		// Return a copy to prevent modification
		result := make([]EthernetInterface, len(ifaces))
		copy(result, ifaces)
		return result, nil
	}
	return []EthernetInterface{}, nil
}

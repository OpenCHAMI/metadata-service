// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"fmt"
	"sync"
)

// MockSMDClient is a mock implementation of SMDClient for testing
type MockSMDClient struct {
	mu         sync.RWMutex
	components map[string]*Component
	ipToID     map[string]string
	groups     map[string][]string // component ID -> group names
	wgip       map[string]string   // component ID -> WG IP
}

// NewMockSMDClient creates a new mock SMD client
func NewMockSMDClient() *MockSMDClient {
	return &MockSMDClient{
		components: make(map[string]*Component),
		ipToID:     make(map[string]string),
		groups:     make(map[string][]string),
		wgip:       make(map[string]string),
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

// AddGroupMembership adds a component to a group
func (m *MockSMDClient) AddGroupMembership(componentID string, groups []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[componentID] = groups
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

// IPfromID returns the IP address for a given component ID
func (m *MockSMDClient) IPfromID(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if component, ok := m.components[id]; ok {
		return component.IP, nil
	}
	return "", fmt.Errorf("no component found for ID %s", id)
}

// MACfromID returns the MAC address for a given component ID
func (m *MockSMDClient) MACfromID(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if component, ok := m.components[id]; ok {
		return component.MAC, nil
	}
	return "", fmt.Errorf("no component found for ID %s", id)
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

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2025 OpenCHAMI a Series of LF Projects, LLC

package wireguard

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// ControllerState represents the persistent state of a WireGuard controller.
type ControllerState struct {
	Version          string                `yaml:"version"`
	ServerPrivateKey string                `yaml:"serverPrivateKey"`
	ServerPublicKey  string                `yaml:"serverPublicKey"`
	AllocatedIPs     []string              `yaml:"allocatedIPs"`
	Peers            []PersistentPeerState `yaml:"peers"`
}

// PersistentPeerState is the serializable form of a peer.
type PersistentPeerState struct {
	PeerID    string `yaml:"peerID"`
	PublicKey string `yaml:"publicKey"`
	VPNIP     string `yaml:"vpnIP"`
	ClientIP  string `yaml:"clientIP"`
	AllowedIP string `yaml:"allowedIP"`
}

// Persistence manages file-based storage of controller state.
type Persistence struct {
	filePath string
	mu       sync.RWMutex
}

// NewPersistence creates a new persistence manager for the given file path.
func NewPersistence(filePath string) (*Persistence, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create persistence directory: %w", err)
	}
	return &Persistence{filePath: filePath}, nil
}

// Load reads and parses the controller state from disk; returns a zero value if file doesn't exist.
func (p *Persistence) Load() (*ControllerState, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data, err := os.ReadFile(p.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet; return empty state
			return &ControllerState{
				Version:      "1",
				AllocatedIPs: []string{},
				Peers:        []PersistentPeerState{},
			}, nil
		}
		return nil, fmt.Errorf("failed to read persistence file: %w", err)
	}

	var state ControllerState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse persistence file: %w", err)
	}

	return &state, nil
}

// Save writes the controller state atomically to disk (write-to-temp, then rename).
func (p *Persistence) Save(state *ControllerState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file in same directory for atomic rename
	dir := filepath.Dir(p.filePath)
	tmpFile, err := os.CreateTemp(dir, ".wg-state-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile.Name(), p.filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

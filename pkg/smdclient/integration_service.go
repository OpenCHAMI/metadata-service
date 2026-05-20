// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package smdclient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultSyncInterval = 5 * time.Minute
	defaultBackoffStart = time.Second
)

// IntegrationOptions configures the cache-backed integration service.
type IntegrationOptions struct {
	SyncEnabled  bool
	SyncInterval time.Duration
}

type nodeSnapshot struct {
	component *Component
	groups    []string
	nics      []EthernetNIC
	ifaces    []EthernetInterface
	wgip      string
}

// SMDIntegrationService is a cache-backed SMD facade with a background sync
// worker and live SMD fallback.
type SMDIntegrationService struct {
	backend      SMDClient
	lister       ComponentLister
	syncEnabled  bool
	syncInterval time.Duration

	mu      sync.RWMutex
	nodes   map[string]nodeSnapshot
	ipToID  map[string]string
	wgipMap map[string]string
	lastRun time.Time
}

// InitialSyncStatus reports whether the initial background sync requirement has
// been satisfied for health checks.
func (s *SMDIntegrationService) InitialSyncStatus() (bool, string) {
	if !s.syncEnabled {
		return true, ""
	}

	s.mu.RLock()
	lastRun := s.lastRun
	s.mu.RUnlock()

	if lastRun.IsZero() {
		return false, "smd initial refresh pending"
	}

	return true, ""
}

// NewSMDIntegrationService constructs a cache-backed SMD integration service.
func NewSMDIntegrationService(backend SMDClient, opts IntegrationOptions) *SMDIntegrationService {
	interval := opts.SyncInterval
	if interval <= 0 {
		interval = defaultSyncInterval
	}

	service := &SMDIntegrationService{
		backend:      backend,
		syncEnabled:  opts.SyncEnabled,
		syncInterval: interval,
		nodes:        make(map[string]nodeSnapshot),
		ipToID:       make(map[string]string),
		wgipMap:      make(map[string]string),
	}

	if lister, ok := backend.(ComponentLister); ok {
		service.lister = lister
	}

	return service
}

// ResolveComponentID resolves a client IP using cache-first wireguard then
// management lookup, with live fallback.
func (s *SMDIntegrationService) ResolveComponentID(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "", fmt.Errorf("empty IP")
	}

	cachedWG, cachedIP, stale := s.resolveFromCache(ip)
	if cachedWG != "" && !stale {
		return cachedWG, nil
	}
	if cachedIP != "" && !stale {
		return cachedIP, nil
	}

	id, err := ResolveComponentID(s.backend, ip)
	if err == nil && id != "" {
		s.cacheIPIndex(ip, id)
		return id, nil
	}

	if cachedWG != "" {
		return cachedWG, nil
	}
	if cachedIP != "" {
		return cachedIP, nil
	}

	return "", err
}

// StartSyncWorker starts the background sync loop. It performs an initial sync
// and then periodic sync attempts. Sync failures are fail-open and retried.
func (s *SMDIntegrationService) StartSyncWorker(ctx context.Context) {
	if !s.syncEnabled {
		return
	}
	go s.runSyncLoop(ctx)
}

func (s *SMDIntegrationService) runSyncLoop(ctx context.Context) {
	if s.lister == nil {
		log.Warn().Msg("SMD sync worker disabled: backend does not support component listing")
		return
	}

	backoff := defaultBackoffStart
	for {
		err := s.syncOnce(ctx)
		if err == nil {
			break
		}
		log.Warn().Err(err).Dur("backoff", backoff).Msg("SMD initial sync failed; retrying")
		if !sleepWithContext(ctx, backoff) {
			return
		}
		backoff *= 2
		if backoff > s.syncInterval {
			backoff = s.syncInterval
		}
	}

	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.syncOnce(ctx); err != nil {
				log.Warn().Err(err).Msg("SMD periodic sync failed")
			}
		}
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *SMDIntegrationService) syncOnce(ctx context.Context) error {
	if s.lister == nil {
		return fmt.Errorf("backend does not support ListComponents")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	components, err := s.lister.ListComponents()
	if err != nil {
		return fmt.Errorf("list components: %w", err)
	}

	nextNodes := make(map[string]nodeSnapshot, len(components))
	nextIPIndex := make(map[string]string)
	nextWGIPIndex := make(map[string]string)

	for _, component := range components {
		if component == nil || component.ID == "" {
			continue
		}

		componentCopy := *component
		snapshot := nodeSnapshot{component: &componentCopy}

		if groups, err := s.backend.GroupMembership(component.ID); err != nil {
			log.Warn().Err(err).Str("component", component.ID).Msg("Failed to fetch group membership during sync")
		} else {
			snapshot.groups = cloneStrings(groups)
		}

		if ifaces, err := s.backend.EthernetInterfaces(component.ID); err != nil {
			log.Warn().Err(err).Str("component", component.ID).Msg("Failed to fetch EthernetInterfaces during sync")
		} else {
			snapshot.ifaces = cloneIfaces(ifaces)
		}

		if nics, err := s.backend.EthernetNICInfo(component.ID); err != nil {
			log.Warn().Err(err).Str("component", component.ID).Msg("Failed to fetch EthernetNICInfo during sync")
		} else {
			snapshot.nics = cloneNICs(nics)
		}

		if wgip, err := s.backend.WGIPfromID(component.ID); err == nil && wgip != "" {
			snapshot.wgip = wgip
			nextWGIPIndex[wgip] = component.ID
		}

		if componentCopy.IP != "" {
			nextIPIndex[componentCopy.IP] = component.ID
		}
		for _, iface := range snapshot.ifaces {
			for _, mapping := range iface.IPAddresses {
				if mapping.IPAddress == "" {
					continue
				}
				nextIPIndex[mapping.IPAddress] = component.ID
			}
		}

		nextNodes[component.ID] = snapshot
	}

	s.mu.Lock()
	s.nodes = nextNodes
	s.ipToID = nextIPIndex
	s.wgipMap = nextWGIPIndex
	s.lastRun = time.Now().UTC()
	s.mu.Unlock()

	log.Debug().Int("nodes", len(nextNodes)).Msg("SMD sync completed")
	return nil
}

func (s *SMDIntegrationService) isCacheStale(lastRun time.Time) bool {
	if !s.syncEnabled {
		return false
	}
	if lastRun.IsZero() {
		return true
	}
	return time.Since(lastRun) > 2*s.syncInterval
}

func (s *SMDIntegrationService) resolveFromCache(ip string) (wgID string, ipID string, stale bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wgID = s.wgipMap[ip]
	ipID = s.ipToID[ip]
	stale = s.isCacheStale(s.lastRun)
	return wgID, ipID, stale
}

func (s *SMDIntegrationService) cacheIPIndex(ip, id string) {
	if ip == "" || id == "" {
		return
	}
	s.mu.Lock()
	s.ipToID[ip] = id
	s.mu.Unlock()
}

func (s *SMDIntegrationService) cacheWGIP(id, wgip string) {
	if id == "" || wgip == "" {
		return
	}
	s.mu.Lock()
	node := s.nodes[id]
	node.wgip = wgip
	s.nodes[id] = node
	s.wgipMap[wgip] = id
	s.mu.Unlock()
}

// IDfromIP returns the component ID for a management IP.
func (s *SMDIntegrationService) IDfromIP(ip string) (string, error) {
	s.mu.RLock()
	cachedID := s.ipToID[ip]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if cachedID != "" && !stale {
		return cachedID, nil
	}

	id, err := s.backend.IDfromIP(ip)
	if err == nil {
		s.cacheIPIndex(ip, id)
		return id, nil
	}
	if cachedID != "" {
		return cachedID, nil
	}
	return "", err
}

// IDfromWGIP returns the component ID for a WireGuard IP.
func (s *SMDIntegrationService) IDfromWGIP(wgip string) (string, error) {
	s.mu.RLock()
	cachedID := s.wgipMap[wgip]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if cachedID != "" && !stale {
		return cachedID, nil
	}

	id, err := s.backend.IDfromWGIP(wgip)
	if err == nil {
		s.cacheWGIP(id, wgip)
		return id, nil
	}
	if cachedID != "" {
		return cachedID, nil
	}
	return "", err
}

// IPfromID returns a component's preferred boot IP.
func (s *SMDIntegrationService) IPfromID(id string) (string, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale {
		if ip := pickHMNIP(node.ifaces); ip != "" {
			return ip, nil
		}
		if node.component != nil && node.component.IP != "" {
			return node.component.IP, nil
		}
	}

	ip, err := s.backend.IPfromID(id)
	if err == nil {
		s.cacheIPIndex(ip, id)
		return ip, nil
	}

	if found {
		if ip := pickHMNIP(node.ifaces); ip != "" {
			return ip, nil
		}
		if node.component != nil && node.component.IP != "" {
			return node.component.IP, nil
		}
	}
	return "", err
}

// MACfromID returns a component's preferred boot MAC.
func (s *SMDIntegrationService) MACfromID(id string) (string, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale {
		if mac := pickHMNMAC(node.ifaces); mac != "" {
			return mac, nil
		}
		if node.component != nil && node.component.MAC != "" {
			return node.component.MAC, nil
		}
	}

	mac, err := s.backend.MACfromID(id)
	if err == nil {
		return mac, nil
	}

	if found {
		if mac := pickHMNMAC(node.ifaces); mac != "" {
			return mac, nil
		}
		if node.component != nil && node.component.MAC != "" {
			return node.component.MAC, nil
		}
	}
	return "", err
}

// ComponentInformation returns component information by ID.
func (s *SMDIntegrationService) ComponentInformation(id string) (*Component, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale && node.component != nil {
		copy := *node.component
		return &copy, nil
	}

	component, err := s.backend.ComponentInformation(id)
	if err == nil {
		s.mu.Lock()
		n := s.nodes[id]
		componentCopy := *component
		n.component = &componentCopy
		s.nodes[id] = n
		if componentCopy.IP != "" {
			s.ipToID[componentCopy.IP] = id
		}
		s.mu.Unlock()
		return component, nil
	}

	if found && node.component != nil {
		copy := *node.component
		return &copy, nil
	}
	return nil, err
}

// GroupMembership returns the list of groups for a component.
func (s *SMDIntegrationService) GroupMembership(id string) ([]string, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale {
		return cloneStrings(node.groups), nil
	}

	groups, err := s.backend.GroupMembership(id)
	if err == nil {
		s.mu.Lock()
		n := s.nodes[id]
		n.groups = cloneStrings(groups)
		s.nodes[id] = n
		s.mu.Unlock()
		return cloneStrings(groups), nil
	}

	if found {
		return cloneStrings(node.groups), nil
	}
	return nil, err
}

// AddWGIP records the allocated WireGuard IP for a component.
func (s *SMDIntegrationService) AddWGIP(id, wgip string) error {
	err := s.backend.AddWGIP(id, wgip)
	if err != nil {
		return err
	}
	s.cacheWGIP(id, wgip)
	return nil
}

// WGIPfromID returns the stored WireGuard IP.
func (s *SMDIntegrationService) WGIPfromID(id string) (string, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale && node.wgip != "" {
		return node.wgip, nil
	}

	wgip, err := s.backend.WGIPfromID(id)
	if err == nil {
		s.cacheWGIP(id, wgip)
		return wgip, nil
	}

	if found && node.wgip != "" {
		return node.wgip, nil
	}
	return "", err
}

// EthernetNICInfo returns NIC data for a component.
func (s *SMDIntegrationService) EthernetNICInfo(id string) ([]EthernetNIC, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale && len(node.nics) > 0 {
		return cloneNICs(node.nics), nil
	}

	nics, err := s.backend.EthernetNICInfo(id)
	if err == nil {
		s.mu.Lock()
		n := s.nodes[id]
		n.nics = cloneNICs(nics)
		s.nodes[id] = n
		s.mu.Unlock()
		return cloneNICs(nics), nil
	}

	if found {
		return cloneNICs(node.nics), nil
	}
	return nil, err
}

// EthernetInterfaces returns interface data for a component.
func (s *SMDIntegrationService) EthernetInterfaces(id string) ([]EthernetInterface, error) {
	s.mu.RLock()
	node, found := s.nodes[id]
	stale := s.isCacheStale(s.lastRun)
	s.mu.RUnlock()
	if found && !stale && len(node.ifaces) > 0 {
		return cloneIfaces(node.ifaces), nil
	}

	ifaces, err := s.backend.EthernetInterfaces(id)
	if err == nil {
		s.mu.Lock()
		n := s.nodes[id]
		n.ifaces = cloneIfaces(ifaces)
		s.nodes[id] = n
		for _, iface := range ifaces {
			for _, mapping := range iface.IPAddresses {
				if mapping.IPAddress != "" {
					s.ipToID[mapping.IPAddress] = id
				}
			}
		}
		s.mu.Unlock()
		return cloneIfaces(ifaces), nil
	}

	if found {
		return cloneIfaces(node.ifaces), nil
	}
	return nil, err
}

func cloneStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneNICs(in []EthernetNIC) []EthernetNIC {
	out := make([]EthernetNIC, len(in))
	copy(out, in)
	return out
}

func cloneIfaces(in []EthernetInterface) []EthernetInterface {
	out := make([]EthernetInterface, len(in))
	for i := range in {
		iface := in[i]
		iface.IPAddresses = append([]IPMapping(nil), iface.IPAddresses...)
		out[i] = iface
	}
	return out
}

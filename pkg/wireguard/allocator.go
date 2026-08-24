// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2025 OpenCHAMI a Series of LF Projects, LLC

// Package wireguard implements userspace WireGuard helpers and utilities.
package wireguard

import (
	"errors"
	"net"
	"sync"
)

// IPAllocator manages sequential IP reservations within a CIDR.
type IPAllocator struct {
	network       *net.IPNet
	used          map[string]bool
	mu            sync.Mutex
	networkAddr   net.IP
	broadcastAddr net.IP
}

// NewIPAllocator constructs an allocator for the provided CIDR.
func NewIPAllocator(cidr string) (*IPAllocator, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	networkAddr := ipnet.IP.Mask(ipnet.Mask)
	broadcastAddr := make(net.IP, len(networkAddr))
	for i := range networkAddr {
		broadcastAddr[i] = networkAddr[i] | ^ipnet.Mask[i]
	}
	return &IPAllocator{
		network:       ipnet,
		used:          make(map[string]bool),
		networkAddr:   networkAddr,
		broadcastAddr: broadcastAddr,
	}, nil
}

// Reserve marks an IP as used if it is within the configured network.
func (a *IPAllocator) Reserve(addr net.IPAddr) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.network.Contains(addr.IP) {
		return errors.New("IP not in subnet")
	}
	a.used[addr.IP.String()] = true
	return nil
}

// NextAvailable returns the next free IP address within the network.
func (a *IPAllocator) NextAvailable() (net.IPAddr, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	start := make(net.IP, len(a.networkAddr))
	copy(start, a.networkAddr)
	// first usable address is network+1
	start[3]++
	for ip := start; a.network.Contains(ip) && !ip.Equal(a.broadcastAddr); ip[3]++ {
		if !a.used[ip.String()] {
			a.used[ip.String()] = true
			return net.IPAddr{IP: ip}, nil
		}
	}
	return net.IPAddr{}, errors.New("no available IPs")
}

// Release frees a previously reserved IP address.
func (a *IPAllocator) Release(addr net.IPAddr) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, addr.IP.String())
	return nil
}

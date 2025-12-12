// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package wireguard

import (
	"errors"
	"net"
	"sync"
)

type IPAllocator struct {
	network       *net.IPNet
	used          map[string]bool
	mu            sync.Mutex
	networkAddr   net.IP
	broadcastAddr net.IP
}

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

func (a *IPAllocator) Reserve(addr net.IPAddr) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.network.Contains(addr.IP) {
		return errors.New("IP not in subnet")
	}
	a.used[addr.IP.String()] = true
	return nil
}

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

func (a *IPAllocator) Release(addr net.IPAddr) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, addr.IP.String())
	return nil
}

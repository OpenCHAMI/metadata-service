// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
)

// populateMockSMDForLoadTest populates the mock SMD client with 10,000 nodes
// matching the node ID/IP generation pattern from load-tests/common.js
//
// Node ID Format: x{1000+cabinet}c{chassis}s{slot}b0n{node}
// IP Format: 10.{1+floor(index/65536)}.{floor((index%65536)/256)}.{index%256}
//
// Example mappings:
//
//	index=0     → x1000c0s0b0n0 → 10.1.0.0
//	index=1     → x1000c0s0b0n1 → 10.1.0.1
//	index=100   → x1000c0s1b0n0 → 10.1.0.100
//	index=1000  → x1001c0s0b0n0 → 10.1.3.232
//	index=9999  → x1009c9s9b0n9 → 10.1.39.15
func populateMockSMDForLoadTest(mock *smdclient.MockSMDClient) {
	const numNodes = 10000

	for i := 0; i < numNodes; i++ {
		// Generate node ID matching load-tests/common.js::generateNodeID()
		cabinet := i / 1000
		chassis := (i % 1000) / 100
		slot := (i % 100) / 10
		node := i % 10
		nodeID := fmt.Sprintf("x%dc%ds%db0n%d", 1000+cabinet, chassis, slot, node)

		// Generate IP matching load-tests/common.js::generateIP()
		octet2 := 1 + (i / 65536)
		octet3 := (i % 65536) / 256
		octet4 := i % 256
		ip := fmt.Sprintf("10.%d.%d.%d", octet2, octet3, octet4)

		// Generate MAC address (simple sequential pattern)
		macOctet5 := (i / 256) % 256
		macOctet6 := i % 256
		mac := fmt.Sprintf("b4:2e:99:be:%02x:%02x", macOctet5, macOctet6)

		// Add component
		mock.AddComponent(&smdclient.Component{
			ID:   nodeID,
			NID:  int64(1000 + i),
			Role: "compute",
			MAC:  mac,
			IP:   ip,
		})

		// Add group membership (all nodes in "compute" group)
		mock.AddGroupMembership(nodeID, []string{"compute"})

		// Add EthernetInterfaces (IP mapping for IDfromIP lookups)
		mock.AddEthernetInterfaces(nodeID, []smdclient.EthernetInterface{
			{
				ID:          fmt.Sprintf("iface-%d", i),
				Description: "Node Management Network",
				MACAddress:  mac,
				IPAddresses: []smdclient.IPMapping{
					{
						IPAddress: ip,
						Network:   "HMN",
					},
				},
				ComponentID: nodeID,
				Type:        "Node",
			},
		})
	}
}

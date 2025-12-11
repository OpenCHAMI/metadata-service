// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

/*
Package smdclient provides an interface and implementations for interacting with
the State Management Database (SMD).

SMD is the central repository for hardware inventory, component status, and group
membership information in OpenCHAMI systems. This package abstracts SMD interaction,
allowing the cloud-init metadata service to retrieve hardware information and verify
node group membership.

# SMDClient Interface

The SMDClient interface defines the contract for SMD interaction:

  - IDfromIP(ip string) (string, error): Resolves an IP address to a component ID
  - IPfromID(id string) (string, error): Retrieves the IP address for a component
  - MACfromID(id string) (string, error): Retrieves the MAC address for a component
  - ComponentInformation(id string) (*Component, error): Gets detailed component info
  - GroupMembership(id string) ([]string, error): Lists groups a component belongs to

# Component Structure

The Component struct represents a hardware component with the following information:

  - ID: Unique component identifier (e.g., "x1000c0s0b0n0")
  - NID: Numeric Node ID in the cluster
  - Role: Component role (e.g., "compute", "login", "storage")
  - MAC: MAC address of the component
  - IP: IP address of the component

# Implementations

The package provides two implementations of the SMDClient interface:

Production Implementation (HTTPClient)

	When SMD_URL environment variable is set, an HTTP client connects to the real SMD instance.
	The HTTP client:
	  - Queries SMD REST API endpoints for component information
	  - Handles SMD connection errors and retries
	  - Caches results to reduce load on SMD
	  - Supports TLS and authentication if configured

Mock Implementation (MockSMDClient)

	When SMD_URL is not set, a mock client is used for development and testing.
	The mock client:
	  - Provides three test nodes: x1000c0s0b0n0, x1000c0s1b0n0, x1000c0s2b0n0
	  - Maps IPs to components (10.0.0.100, 10.0.0.101, 10.0.0.102)
	  - Returns realistic component data for testing
	  - Provides group memberships (compute, login groups)

The mock client enables development without requiring a running SMD instance,
simplifying local testing and demonstration workflows.

# Usage in Handlers

The cloud-init handlers use SMDClient to:

1. Resolve client IP to component ID via IDfromIP()
2. Retrieve detailed component information via ComponentInformation()
3. Verify group membership via GroupMembership() for group-specific endpoints
4. Extract template variables (hostname, nid, role) from component data

Example handler usage:

	client := smdclient.NewSMDClient()  // Uses mock or HTTP based on env vars
	componentID, err := client.IDfromIP("10.0.0.100")
	if err != nil {
	    // Handle not found or connection error
	}
	component, err := client.ComponentInformation(componentID)
	if err != nil {
	    // Handle error
	}
	// Use component.Hostname, component.NID, component.Role in templates

# Interface-Based Design

The SMDClient interface enables:

  - Easy testing: Mock implementation for unit and integration tests
  - Production flexibility: Different implementations without changing handler code
  - Development efficiency: Mock client eliminates dependency on SMD during development
  - Clear contracts: Interface defines exactly what SMD operations are required

# Creating a Custom Implementation

To create a custom SMD implementation (e.g., for a different backend):

1. Create a new type implementing the SMDClient interface
2. Implement all five required methods
3. Create a factory function or initialization logic to select your implementation
4. Update initialization code in cmd/server/smd.go as needed
5. No changes needed to handlers - they use only the interface

Example custom implementation:

	type PostgresSMDClient struct {
	    conn *sql.DB
	}

	func (c *PostgresSMDClient) IDfromIP(ip string) (string, error) {
	    // Query Postgres database
	    var id string
	    err := c.conn.QueryRow(
	        "SELECT component_id FROM components WHERE ip = $1", ip,
	    ).Scan(&id)
	    return id, err
	}

	// Implement remaining methods...

# Environment Variables

SMD_URL: SMD base URL for production use

	Example: SMD_URL=http://smd.example.com:27779
	When unset, mock client is automatically used

SMD_TIMEOUT: HTTP client timeout for SMD requests (default: 10s)
SMD_RETRIES: Number of retry attempts for failed requests (default: 3)

# Error Handling

SMDClient methods return errors in these cases:

  - Component not found: ErrNotFound
  - Network/connection error: Underlying error from HTTP client
  - Invalid input: ErrInvalidInput
  - SMD service error: HTTP error responses

Handlers should implement graceful degradation when SMD is unavailable, such as
returning 503 Service Unavailable with appropriate error messages.

# Caching Considerations

For high-throughput scenarios, consider implementing caching at the SMDClient level:
  - Cache ComponentInformation results with TTL
  - Cache GroupMembership with shorter TTL (groups change less frequently)
  - Implement cache invalidation strategies

The current implementation prioritizes correctness over performance; production
deployments should evaluate caching needs based on traffic patterns.

See cmd/server/smd.go for initialization logic and example factory patterns.
*/
package smdclient

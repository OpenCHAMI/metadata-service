// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

// Common utilities and configuration for k6 load tests

import { check } from 'k6';

// Configuration
export const config = {
  baseURL: __ENV.METADATA_SERVICE_URL || 'http://localhost:8080',
  numNodes: parseInt(__ENV.NUM_NODES || '10000'),
  mockSMD: __ENV.MOCK_SMD !== 'false', // Default true
};

// Generate a realistic OpenCHAMI node ID
export function generateNodeID(index) {
  const cabinet = Math.floor(index / 1000);
  const chassis = Math.floor((index % 1000) / 100);
  const slot = Math.floor((index % 100) / 10);
  const node = index % 10;
  return `x${1000 + cabinet}c${chassis}s${slot}b0n${node}`;
}

// Generate IP address from node index
export function generateIP(index) {
  const octet2 = 1 + Math.floor(index / 65536);
  const octet3 = Math.floor((index % 65536) / 256);
  const octet4 = index % 256;
  return `10.${octet2}.${octet3}.${octet4}`;
}

// Standard checks for metadata endpoints
export function checkMetadataResponse(response, maxLatency = 2000) {
  return check(response, {
    'status is 200': (r) => r.status === 200,
    'response time < threshold': (r) => r.timings.duration < maxLatency,
    'has content': (r) => r.body && r.body.length > 0,
    'not error page': (r) => !r.body.includes('error') && !r.body.includes('Error'),
  });
}

// Cloud-init NoCloud endpoints
export const endpoints = {
  metaData: '/meta-data',
  userData: '/user-data',
  vendorData: '/vendor-data',
  networkConfig: '/network-config',
};

// Request headers for a given node
export function nodeHeaders(nodeIndex) {
  return {
    headers: {
      'X-Forwarded-For': generateIP(nodeIndex),
      'User-Agent': 'cloud-init/24.1',
    },
  };
}

// Generate a random node index
export function randomNodeIndex() {
  return Math.floor(Math.random() * config.numNodes);
}

// VU-specific node index (consistent per VU)
export function vuNodeIndex() {
  return __VU % config.numNodes;
}

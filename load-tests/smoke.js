// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

// Smoke Test: Quick validation with minimal load
// Purpose: Verify service is functional before running larger tests
// Duration: 30 seconds
// Load: 10 concurrent virtual users

import http from 'k6/http';
import { sleep } from 'k6';
import { config, endpoints, nodeHeaders, vuNodeIndex, checkMetadataResponse } from './common.js';

export let options = {
  vus: 10,
  duration: '30s',
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.01'], // <1% failure
    checks: ['rate>0.95'], // >95% checks pass
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export default function () {
  const nodeIndex = vuNodeIndex();
  const headers = nodeHeaders(nodeIndex);

  // Test each cloud-init endpoint
  let res;

  // /meta-data
  res = http.get(`${config.baseURL}${endpoints.metaData}`, headers);
  checkMetadataResponse(res, 1000);

  sleep(0.1);

  // /user-data
  res = http.get(`${config.baseURL}${endpoints.userData}`, headers);
  checkMetadataResponse(res, 1000);

  sleep(0.1);

  // /vendor-data
  res = http.get(`${config.baseURL}${endpoints.vendorData}`, headers);
  checkMetadataResponse(res, 1000);

  sleep(1); // Think time between iterations
}

export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'load-tests/results/smoke-summary.json': JSON.stringify(data),
  };
}

function textSummary(data, options) {
  // Basic summary formatter
  const metrics = data.metrics;
  const indent = options.indent || '';
  const enableColors = options.enableColors || false;

  let output = '\n';
  output += `${indent}Test: Smoke Test\n`;
  output += `${indent}Duration: ${options.vus || 10} VUs for 30s\n\n`;

  if (metrics.http_req_duration) {
    const d = metrics.http_req_duration.values;
    output += `${indent}Response Time:\n`;
    output += `${indent}  avg=${d.avg.toFixed(2)}ms  p(95)=${d['p(95)'].toFixed(2)}ms  p(99)=${d['p(99)'].toFixed(2)}ms\n`;
  }

  if (metrics.http_req_failed) {
    const failed = metrics.http_req_failed.values.rate * 100;
    output += `${indent}Failed Requests: ${failed.toFixed(2)}%\n`;
  }

  if (metrics.checks) {
    const passed = metrics.checks.values.rate * 100;
    output += `${indent}Checks Passed: ${passed.toFixed(2)}%\n`;
  }

  output += '\n';
  return output;
}

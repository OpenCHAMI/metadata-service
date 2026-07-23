// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

// Cold Boot Storm Test: Worst-case scenario
// Purpose: Simulate datacenter power-on - all nodes boot simultaneously
// Duration: ~3 minutes
// Load: 0 → 10K VUs in 30 seconds (the "thundering herd")

import http from 'k6/http';
import { sleep } from 'k6';
import { config, endpoints, nodeHeaders, vuNodeIndex, checkMetadataResponse } from './common.js';

export let options = {
  stages: [
    { duration: '10s', target: 2000 },  // Initial ramp
    { duration: '20s', target: 10000 }, // THE STORM ⚡
    { duration: '60s', target: 10000 }, // Sustained peak load
    { duration: '30s', target: 5000 },  // Partial recovery
    { duration: '30s', target: 0 },     // Full recovery
  ],
  thresholds: {
    // Production targets for worst-case scenario (10K concurrent boot)
    http_req_duration: ['p(95)<3000', 'p(99)<5000'],
    http_req_failed: ['rate<0.01'], // <1% failure rate
    'http_req_duration{endpoint:meta-data}': ['p(99)<4000'],

    // Resource exhaustion indicators
    'http_req_connecting': ['p(99)<100'], // Connection time should be fast
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
};

export default function () {
  const nodeIndex = vuNodeIndex();
  const headers = nodeHeaders(nodeIndex);

  // During boot storm, nodes hit endpoints in parallel (batch request)
  const responses = http.batch([
    {
      method: 'GET',
      url: `${config.baseURL}${endpoints.metaData}`,
      params: { ...headers, tags: { endpoint: 'meta-data' } },
    },
    {
      method: 'GET',
      url: `${config.baseURL}${endpoints.userData}`,
      params: { ...headers, tags: { endpoint: 'user-data' } },
    },
    {
      method: 'GET',
      url: `${config.baseURL}${endpoints.vendorData}`,
      params: { ...headers, tags: { endpoint: 'vendor-data' } },
    },
  ]);

  // Check all responses
  responses.forEach((res, idx) => {
    const endpoint = ['meta-data', 'user-data', 'vendor-data'][idx];
    checkMetadataResponse(res, 5000);
  });

  // Short think time during storm
  sleep(0.5);
}

export function handleSummary(data) {
  console.log('\n========================================');
  console.log('⚡ COLD BOOT STORM TEST RESULTS ⚡');
  console.log('========================================');
  console.log('Scenario: All 10,000 nodes boot simultaneously');
  console.log('Duration: ~3 minutes');
  console.log('');

  const metrics = data.metrics;

  if (metrics.http_req_duration) {
    const d = metrics.http_req_duration.values;
    console.log('Response Time:');
    console.log(`  avg:    ${d.avg.toFixed(2)}ms`);
    console.log(`  median: ${d.med.toFixed(2)}ms`);
    console.log(`  p(90):  ${d['p(90)'].toFixed(2)}ms`);
    console.log(`  p(95):  ${d['p(95)'].toFixed(2)}ms`);
    console.log(`  p(99):  ${d['p(99)'].toFixed(2)}ms`);
    console.log(`  p(99.9):${d['p(99.9)'].toFixed(2)}ms`);
    console.log(`  max:    ${d.max.toFixed(2)}ms`);
  }

  if (metrics.http_req_failed) {
    const failed = metrics.http_req_failed.values.rate * 100;
    const total = metrics.http_reqs ? metrics.http_reqs.values.count : 0;
    const failedCount = Math.floor(failed * total / 100);

    console.log('');
    if (failed > 5) {
      console.log(`❌ Failed Requests: ${failed.toFixed(2)}% (${failedCount} of ${total})`);
    } else if (failed > 1) {
      console.log(`⚠️  Failed Requests: ${failed.toFixed(2)}% (${failedCount} of ${total})`);
      console.log('   ^ Acceptable for storm, but should investigate');
    } else {
      console.log(`✅ Failed Requests: ${failed.toFixed(2)}% (${failedCount} of ${total})`);
    }
  }

  if (metrics.checks) {
    const passed = metrics.checks.values.rate * 100;
    console.log(`Checks Passed: ${passed.toFixed(2)}%`);
  }

  // Connection time analysis
  if (metrics.http_req_connecting) {
    const c = metrics.http_req_connecting.values;
    console.log('');
    console.log('Connection Time (TCP handshake):');
    console.log(`  avg: ${c.avg.toFixed(2)}ms  p(99): ${c['p(99)'].toFixed(2)}ms`);
    if (c['p(99)'] > 100) {
      console.log('  ⚠️  High connection time - possible socket exhaustion');
    }
  }

  // Per-endpoint breakdown
  console.log('');
  console.log('Per-Endpoint P99 Latency:');
  ['meta-data', 'user-data', 'vendor-data'].forEach(ep => {
    const key = `http_req_duration{endpoint:${ep}}`;
    if (metrics[key]) {
      const p99 = metrics[key].values['p(99)'];
      const status = p99 < 2000 ? '✅' : p99 < 5000 ? '⚠️' : '❌';
      console.log(`  ${status} /${ep}: ${p99.toFixed(2)}ms`);
    }
  });

  console.log('');
  console.log('Success Criteria:');
  console.log('  ✅ P99 < 5s');
  console.log('  ✅ Failure rate < 1%');
  console.log('========================================\n');

  return {
    'results/boot-storm-summary.json': JSON.stringify(data),
  };
}

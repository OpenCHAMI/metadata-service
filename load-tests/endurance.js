// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

// Endurance Test: Sustained load for memory leak detection
// Purpose: Find resource leaks (memory, goroutines, file descriptors)
// Duration: 10 minutes
// Load: Constant 2K VUs

import http from 'k6/http';
import { sleep } from 'k6';
import { config, endpoints, nodeHeaders, randomNodeIndex, checkMetadataResponse } from './common.js';

export let options = {
  vus: 2000,
  duration: '10m',
  thresholds: {
    http_req_duration: ['p(95)<1000', 'p(99)<1500'],
    http_req_failed: ['rate<0.001'], // <0.1% failure
    checks: ['rate>0.99'], // >99% checks pass
  },
};

export default function () {
  // Use random node each iteration to prevent cache bias
  const nodeIndex = randomNodeIndex();
  const headers = nodeHeaders(nodeIndex);

  // Single endpoint per iteration to maximize throughput
  const endpoint = endpoints.metaData;

  const res = http.get(
    `${config.baseURL}${endpoint}`,
    headers
  );

  checkMetadataResponse(res, 1500);

  sleep(1); // 2K VUs × 1s sleep = 2K RPS
}

export function handleSummary(data) {
  console.log('\n========================================');
  console.log('Endurance Test Results');
  console.log('========================================');
  console.log('Duration: 10 minutes');
  console.log('Load: 2,000 concurrent VUs');
  console.log('');

  const metrics = data.metrics;

  if (metrics.http_req_duration) {
    const d = metrics.http_req_duration.values;
    console.log('Response Time:');
    console.log(`  avg: ${d.avg.toFixed(2)}ms`);
    console.log(`  p(95): ${d['p(95)'].toFixed(2)}ms`);
    console.log(`  p(99): ${d['p(99)'].toFixed(2)}ms`);
  }

  if (metrics.http_reqs) {
    const total = metrics.http_reqs.values.count;
    const rate = metrics.http_reqs.values.rate;
    console.log('');
    console.log(`Total Requests: ${total}`);
    console.log(`Request Rate: ${rate.toFixed(2)} req/s`);
  }

  if (metrics.http_req_failed) {
    const failed = metrics.http_req_failed.values.rate * 100;
    console.log(`Failed Requests: ${failed.toFixed(4)}%`);
  }

  console.log('');
  console.log('📊 Next: Check for resource leaks');
  console.log('   - Memory: ps aux | grep metadata-service');
  console.log('   - Goroutines: curl http://localhost:6060/debug/pprof/goroutine?debug=1');
  console.log('   - File descriptors: lsof -p $(pgrep metadata-service) | wc -l');
  console.log('========================================\n');

  return {
    'results/endurance-summary.json': JSON.stringify(data),
  };
}

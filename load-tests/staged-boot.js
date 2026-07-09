// Staged Boot Test: Simulates rolling boot waves
// Purpose: Realistic boot scenario - nodes boot in waves, not all at once
// Duration: ~5 minutes
// Load: Ramps 0 → 1K → 2K → 5K → 10K → 0

import http from 'k6/http';
import { sleep } from 'k6';
import { config, endpoints, nodeHeaders, vuNodeIndex, checkMetadataResponse } from './common.js';

export let options = {
  stages: [
    { duration: '30s', target: 1000 },  // Wave 1: 1K nodes
    { duration: '30s', target: 1000 },  // Hold
    { duration: '30s', target: 2000 },  // Wave 2: 2K nodes
    { duration: '30s', target: 2000 },  // Hold
    { duration: '30s', target: 5000 },  // Wave 3: 5K nodes
    { duration: '60s', target: 5000 },  // Hold longer
    { duration: '30s', target: 10000 }, // Wave 4: 10K nodes (final)
    { duration: '60s', target: 10000 }, // Sustained peak
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<1500', 'p(99)<2000'],
    http_req_failed: ['rate<0.01'], // <1% failure
    'http_req_duration{endpoint:meta-data}': ['p(99)<1500'],
    'http_req_duration{endpoint:user-data}': ['p(99)<2000'],
  },
};

export default function () {
  const nodeIndex = vuNodeIndex();
  const headers = nodeHeaders(nodeIndex);
  
  // Simulate cloud-init NoCloud sequence
  // Each node hits these endpoints in order during boot
  
  // 1. /meta-data (required, always first)
  let res = http.get(
    `${config.baseURL}${endpoints.metaData}`,
    { ...headers, tags: { endpoint: 'meta-data' } }
  );
  checkMetadataResponse(res, 1500);
  
  sleep(0.05); // Small delay between requests (realistic)
  
  // 2. /user-data (cloud-config)
  res = http.get(
    `${config.baseURL}${endpoints.userData}`,
    { ...headers, tags: { endpoint: 'user-data' } }
  );
  checkMetadataResponse(res, 2000);
  
  sleep(0.05);
  
  // 3. /vendor-data (cluster defaults)
  res = http.get(
    `${config.baseURL}${endpoints.vendorData}`,
    { ...headers, tags: { endpoint: 'vendor-data' } }
  );
  checkMetadataResponse(res, 2000);
  
  // 4. /network-config (optional, not all nodes request this)
  if (Math.random() < 0.3) { // 30% of nodes
    sleep(0.05);
    res = http.get(
      `${config.baseURL}${endpoints.networkConfig}`,
      { ...headers, tags: { endpoint: 'network-config' } }
    );
    checkMetadataResponse(res, 2000);
  }
  
  // Think time: cloud-init processes metadata before next iteration
  sleep(1);
}

export function handleSummary(data) {
  console.log('\n========================================');
  console.log('Staged Boot Test Results');
  console.log('========================================');
  console.log('Stages: 1K → 2K → 5K → 10K nodes');
  console.log('Duration: ~5 minutes');
  console.log('');
  
  const metrics = data.metrics;
  
  if (metrics.http_req_duration) {
    const d = metrics.http_req_duration.values;
    console.log('Response Time:');
    console.log(`  avg: ${d.avg.toFixed(2)}ms`);
    console.log(`  p(90): ${d['p(90)'].toFixed(2)}ms`);
    console.log(`  p(95): ${d['p(95)'].toFixed(2)}ms`);
    console.log(`  p(99): ${d['p(99)'].toFixed(2)}ms`);
  }
  
  if (metrics.http_req_failed) {
    const failed = metrics.http_req_failed.values.rate * 100;
    const total = metrics.http_reqs ? metrics.http_reqs.values.count : 0;
    console.log('');
    console.log(`Failed Requests: ${failed.toFixed(2)}% (${Math.floor(failed * total / 100)} of ${total})`);
  }
  
  if (metrics.checks) {
    const passed = metrics.checks.values.rate * 100;
    console.log(`Checks Passed: ${passed.toFixed(2)}%`);
  }
  
  // Per-endpoint breakdown
  console.log('');
  console.log('Per-Endpoint P99 Latency:');
  ['meta-data', 'user-data', 'vendor-data'].forEach(ep => {
    const key = `http_req_duration{endpoint:${ep}}`;
    if (metrics[key]) {
      const p99 = metrics[key].values['p(99)'];
      console.log(`  /${ep}: ${p99.toFixed(2)}ms`);
    }
  });
  
  console.log('========================================\n');
  
  return {
    'load-tests/results/staged-boot-summary.json': JSON.stringify(data),
  };
}

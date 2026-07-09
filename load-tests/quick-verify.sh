#!/bin/bash
# Quick verification test - run just the smoke test to verify no regressions

set -e

cd "$(dirname "$0")/.."

echo "🔧 Quick Smoke Test - Post Async-Persistence Fix"
echo "================================================"
echo ""

# Build
echo "Building..."
make build > /dev/null 2>&1

# Start server
echo "Starting server..."
./bin/ochami-metadata-server serve --mock-smd --data-dir /tmp/metadata-service-test --debug > /tmp/metadata-test.log 2>&1 &
SERVER_PID=$!
sleep 3

# Verify server is up
if ! curl -sf -H "X-Forwarded-For: 10.1.0.0" http://localhost:8080/meta-data > /dev/null; then
    echo "❌ Server not responding"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

echo "✅ Server ready"
echo ""

# Run smoke test
echo "Running smoke test (30 seconds)..."
cd load-tests
k6 run smoke.js 2>&1 | tee /tmp/k6-smoke-post-fix.log | grep -E "(✓|✗|http_req_duration|http_req_failed|checks)"
cd ..

# Cleanup
echo ""
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "✅ Smoke test complete!"
echo "Full results: /tmp/k6-smoke-post-fix.log"

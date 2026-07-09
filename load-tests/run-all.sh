#!/bin/bash

# SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
#
# SPDX-License-Identifier: MIT

# Test runner for load tests - fixed and ready to use

set -e

cd "$(dirname "$0")/.."

echo "🚀 Metadata Service Load Test Runner"
echo "======================================"
echo ""

# Build
echo "📦 Building metadata-service..."
make build
echo ""

# Check k6
if ! command -v k6 &> /dev/null; then
    echo "📦 Installing k6..."
    brew install k6
fi

# Start server
echo "🔧 Starting metadata-service with mock SMD (10K nodes)..."
./bin/ochami-metadata-server serve --mock-smd --data-dir /tmp/metadata-service-loadtest --debug > /tmp/metadata-service.log 2>&1 &
SERVER_PID=$!
echo "   PID: $SERVER_PID"
echo "   Logs: tail -f /tmp/metadata-service.log"
echo ""

# Wait for server
echo "⏳ Waiting for server to be ready..."
sleep 3

# Verify
echo "✅ Verifying mock SMD populated..."
if curl -sf -H "X-Forwarded-For: 10.1.0.0" http://localhost:8080/meta-data | grep -q "x1000c0s0b0n0"; then
    echo "   ✅ Node x1000c0s0b0n0 found!"
else
    echo "   ❌ Server not responding correctly"
    kill $SERVER_PID
    exit 1
fi
echo ""

# Function to run test and show results
run_test() {
    local test_name=$1
    local test_file=$2

    echo "=========================================="
    echo "Running: $test_name"
    echo "=========================================="
    cd load-tests
    k6 run "$test_file" 2>&1 | tee "/tmp/k6-$test_name.log"
    cd ..
    echo ""
}

# Run tests
run_test "smoke" "smoke.js"

read -p "Continue to staged boot test? [y/N] " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    run_test "staged-boot" "staged-boot.js"
fi

read -p "Continue to boot storm test (will likely fail)? [y/N] " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "⚠️  WARNING: Boot storm test expected to fail with 30-50% error rate"
    echo "   This demonstrates the WireGuard lock contention issue."
    sleep 2
    run_test "boot-storm" "boot-storm.js"
fi

# Cleanup
echo ""
echo "=========================================="
echo "Cleaning up..."
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true
echo ""

echo "✅ Tests complete!"
echo ""
echo "Results:"
echo "  - Server logs: /tmp/metadata-service.log"
echo "  - Test logs: /tmp/k6-*.log"
echo "  - Test results: load-tests/results/*.json"
echo ""
echo "Next steps:"
echo "  1. Review test output above"
echo "  2. Capture mutex profile:"
echo "     curl http://localhost:6060/debug/pprof/mutex > baseline-mutex.prof"
echo "  3. If boot storm failed (expected), implement async persistence fix"

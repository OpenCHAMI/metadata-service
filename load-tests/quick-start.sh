#!/bin/bash
# Quick start script for running load tests against local metadata-service

set -e

echo "🚀 Metadata Service Load Test Quick Start"
echo "=========================================="
echo ""

# Check if k6 is installed
if ! command -v k6 &> /dev/null; then
    echo "📦 Installing k6..."
    if command -v brew &> /dev/null; then
        brew install k6
    else
        echo "❌ Homebrew not found. Please install k6 manually:"
        echo "   https://k6.io/docs/getting-started/installation/"
        exit 1
    fi
fi

echo "✅ k6 installed: $(k6 version)"
echo ""

# Check if metadata-service is running
if ! curl -s http://localhost:8080/meta-data > /dev/null 2>&1; then
    echo "⚠️  metadata-service not responding on localhost:8080"
    echo ""
    echo "Start the service first:"
    echo "  Terminal 1: cd ~/Development/OpenCHAMI/metadata-service"
    echo "              make build"
    echo "              ./bin/ochami-metadata-server serve --mock-smd --data-dir /tmp/metadata-service --debug"
    echo ""
    echo "  Terminal 2: cd ~/Development/OpenCHAMI/metadata-service"
    echo "              ./load-tests/quick-start.sh"
    echo ""
    exit 1
fi

echo "✅ metadata-service is running"
echo ""

# Create results directory
mkdir -p load-tests/results

# Run smoke test first
echo "1️⃣  Running smoke test (30 seconds)..."
echo "   This validates basic functionality with 10 concurrent users"
echo ""
k6 run load-tests/smoke.js
echo ""

read -p "Continue to staged boot test? (y/n) " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "2️⃣  Running staged boot test (~5 minutes)..."
    echo "   Simulates rolling boot: 1K → 2K → 5K → 10K nodes"
    echo ""
    k6 run load-tests/staged-boot.js
    echo ""
fi

read -p "Continue to boot storm test? (y/n) " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "3️⃣  Running cold boot storm test (~3 minutes)..."
    echo "   ⚠️  WARNING: This will likely fail with high error rate pre-fix"
    echo "   Expected: 30-50% failure due to WireGuard lock contention"
    echo ""
    k6 run load-tests/boot-storm.js
    echo ""
fi

echo "=========================================="
echo "✅ Load tests complete!"
echo ""
echo "Results saved to: load-tests/results/"
echo ""
echo "Next steps:"
echo "  1. Review test output above"
echo "  2. Check profiling data:"
echo "     curl http://localhost:6060/debug/pprof/mutex > mutex.prof"
echo "     go tool pprof -http=:8081 mutex.prof"
echo "  3. If boot storm failed (expected), implement async persistence fix"
echo "  4. Re-run tests to verify improvement"

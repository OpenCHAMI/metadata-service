#!/bin/bash
# quick-test.sh - Quick test of the API with minimal resources
set -e

SERVER_URL="http://localhost:8888"

echo "Quick API Test"
echo "=============="
echo ""

# Check server health
if ! curl -s "${SERVER_URL}/health" > /dev/null 2>&1; then
    echo "❌ Server not running on ${SERVER_URL}"
    echo "Start with: go run ./cmd/server serve --port 8888"
    exit 1
fi
echo "✓ Server is running"
echo ""

# Test cloud-init endpoints with mock nodes
echo "Testing cloud-init endpoints with mock node (10.0.0.100):"
echo ""

echo "1. /meta-data:"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/meta-data" | head -15
echo "..."
echo ""

echo "2. /vendor-data:"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/vendor-data"
echo ""

echo "3. /user-data:"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/user-data"
echo ""

echo "✓ All endpoints responding!"
echo ""
echo "Run ./demo.sh for full demonstration with template creation"

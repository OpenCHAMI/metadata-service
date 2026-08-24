#!/bin/bash

# SPDX-FileCopyrightText: © 2025 OpenCHAMI a Series of LF Projects, LLC
#
# SPDX-License-Identifier: MIT

# quick-test.sh - Quick test of the API with minimal resources
set -euo pipefail

SERVER_URL="${SERVER_URL:-http://localhost:8888}"
MOCK_IP="${MOCK_IP:-10.252.0.26}"

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

# Test cloud-init endpoints with mock node
echo "Testing cloud-init endpoints with mock node (${MOCK_IP}):"
echo ""

echo "1. /meta-data:"
curl -s -H "X-Forwarded-For: ${MOCK_IP}" "${SERVER_URL}/meta-data" | head -15
echo "..."
echo ""

echo "2. /user-data:"
curl -s -H "X-Forwarded-For: ${MOCK_IP}" "${SERVER_URL}/user-data"
echo ""

echo "3. /network-config:"
curl -s -H "X-Forwarded-For: ${MOCK_IP}" "${SERVER_URL}/network-config"
echo ""

echo "✓ All endpoints responding!"
echo ""
echo "Run ./demo.sh for full demonstration with resource creation and rendered group templates"

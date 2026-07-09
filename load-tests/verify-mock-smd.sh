#!/bin/bash

# SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
#
# SPDX-License-Identifier: MIT

# Quick verification that mock SMD was populated correctly

echo "Testing mock SMD population..."
echo ""

# Start server in background
./bin/ochami-metadata-server serve --mock-smd --data-dir /tmp/metadata-service &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Test a few node IDs that should exist
echo "Testing node lookups:"
echo ""

# Test node 0
echo "1. Testing x1000c0s0b0n0 (IP: 10.1.0.0)"
curl -s -H "X-Forwarded-For: 10.1.0.0" http://localhost:8080/meta-data | head -5
echo ""

# Test node 100
echo "2. Testing x1000c0s1b0n0 (IP: 10.1.0.100)"
curl -s -H "X-Forwarded-For: 10.1.0.100" http://localhost:8080/meta-data | head -5
echo ""

# Test node 9999
echo "3. Testing x1009c9s9b0n9 (IP: 10.1.39.15)"
curl -s -H "X-Forwarded-For: 10.1.39.15" http://localhost:8080/meta-data | head -5
echo ""

# Cleanup
kill $SERVER_PID
echo ""
echo "Mock SMD verification complete!"

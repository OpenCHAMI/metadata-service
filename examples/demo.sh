#!/bin/bash

# SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
#
# SPDX-License-Identifier: MIT

# demo.sh - Demonstrates the cloud-init metadata API using the generated client
set -euo pipefail

# Configuration
SERVER_URL="${SERVER_URL:-http://localhost:8888}"
CLIENT="../cmd/client/main.go"
NODE0_IP="10.252.0.26"
NODE1_IP="10.252.0.27"
NODE2_IP="10.252.0.28"

echo "=========================================="
echo "Cloud-Init Metadata API Demo"
echo "=========================================="
echo ""

# Function to wait for server
wait_for_server() {
    echo "Waiting for server to be ready..."
    for i in {1..30}; do
        if curl -s "${SERVER_URL}/health" > /dev/null 2>&1; then
            echo "✓ Server is ready"
            return 0
        fi
        sleep 1
    done
    echo "✗ Server failed to start"
    exit 1
}

wait_for_server
echo ""

echo "=========================================="
echo "1. Creating ClusterDefaults"
echo "=========================================="
cat > /tmp/cluster-defaults.json <<EOF
{
  "metadata": {
    "name": "production-cluster"
  },
  "spec": {
    "description": "Production cluster defaults",
    "base_url": "${SERVER_URL}",
    "cloud_provider": "OpenCHAMI",
    "region": "us-west-1",
    "availability_zone": "us-west-1a",
    "cluster_name": "production",
    "short_name": "prod",
    "nid_length": 6,
    "public_keys": [
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC... admin@production",
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... deploy@automation"
    ]
  }
}
EOF

echo "Creating cluster defaults..."
go run $CLIENT --server "${SERVER_URL}" clusterdefaults create --spec "$(cat /tmp/cluster-defaults.json)"
echo ""

echo "=========================================="
echo "2. Creating Compute Group Template"
echo "=========================================="
cat > /tmp/compute-group.json << 'EOF'
{
  "metadata": {
    "name": "compute"
  },
  "spec": {
    "description": "Standard compute node configuration",
    "template": "#cloud-config\nhostname: {{ ds.hostname }}\nfqdn: {{ ds.hostname }}.{{ ds.cluster_name }}.local\npackages:\n  - htop\n  - jq\nwrite_files:\n  - path: /etc/node-info\n    content: |\n      NODE_ID={{ ds.instance_id }}\n      NODE_NID={{ ds.nid }}\n      NODE_ROLE={{ ds.role }}\n      CLUSTER={{ ds.cluster_name }}\n      PRIMARY_IP={{ ds.ip }}\n",
    "metaData": {
      "scheduler": "slurm",
      "monitoring": "prometheus"
    }
  }
}
EOF

echo "Creating compute group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/compute-group.json)"
echo ""

echo "=========================================="
echo "3. Creating Green Group Template"
echo "=========================================="
cat > /tmp/green-group.json << 'EOF'
{
  "metadata": {
    "name": "green"
  },
  "spec": {
    "description": "Green node overlay",
    "template": "#cloud-config\nwrite_files:\n  - path: /etc/node-color\n    content: |\n      COLOR={{ color }}\n      HOSTNAME={{ ds.hostname }}\n",
    "metaData": {
      "color": "green"
    }
  }
}
EOF

echo "Creating green group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/green-group.json)"
echo ""

echo "=========================================="
echo "4. Creating Blue Group Template"
echo "=========================================="
cat > /tmp/blue-group.json << 'EOF'
{
  "metadata": {
    "name": "blue"
  },
  "spec": {
    "description": "Blue node overlay",
    "template": "#cloud-config\nwrite_files:\n  - path: /etc/node-color\n    content: |\n      COLOR={{ color }}\n      ROLE={{ ds.role }}\n",
    "metaData": {
      "color": "blue"
    }
  }
}
EOF

echo "Creating blue group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/blue-group.json)"
echo ""

echo "=========================================="
echo "5. Creating Storage Group Template"
echo "=========================================="
cat > /tmp/storage-group.json << 'EOF'
{
  "metadata": {
    "name": "storage"
  },
  "spec": {
    "description": "Storage node configuration",
    "template": "#cloud-config\nhostname: {{ ds.hostname }}\nwrite_files:\n  - path: /etc/storage-node\n    content: |\n      NODE={{ ds.instance_id }}\n      ROLE={{ ds.role }}\n      IP={{ ds.ip }}\npackages:\n  - nfs-common\n  - jq\n",
    "metaData": {
      "storage_role": "primary"
    }
  }
}
EOF

echo "Creating storage group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/storage-group.json)"
echo ""

echo "=========================================="
echo "6. Retrieving All Resources"
echo "=========================================="
echo ""

echo "Listing all ClusterDefaults:"
go run $CLIENT --server "${SERVER_URL}" clusterdefaults list
echo ""

echo "Listing all Groups:"
go run $CLIENT --server "${SERVER_URL}" group list
echo ""

echo "=========================================="
echo "7. Viewing Created Resources"
echo "=========================================="
echo ""

echo "All groups have been created successfully!"
echo "You can view them in the list above."
echo "Note: Use the UID (not name) to retrieve specific resources."
echo ""

echo "=========================================="
echo "8. Testing Cloud-Init Endpoints"
echo "=========================================="
echo ""

echo "Testing /meta-data endpoint (simulating node ${NODE0_IP}):"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/meta-data" | head -20
echo "..."
echo ""

echo "Testing /vendor-data endpoint (simulating node ${NODE0_IP}):"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/vendor-data"
echo ""

echo "Testing /user-data endpoint:"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/user-data"
echo ""

echo "Testing /network-config endpoint (simulating node ${NODE0_IP}):"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/network-config"
echo ""

echo "Testing /compute.yaml endpoint (requires group membership):"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/compute.yaml"
echo ""

echo "Testing /green.yaml endpoint (node ${NODE0_IP}):"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/green.yaml"
echo ""

echo "Testing /blue.yaml endpoint (node ${NODE1_IP}):"
curl -s -H "X-Forwarded-For: ${NODE1_IP}" "${SERVER_URL}/blue.yaml"
echo ""

echo "Testing /storage.yaml endpoint (node ${NODE2_IP}):"
curl -s -H "X-Forwarded-For: ${NODE2_IP}" "${SERVER_URL}/storage.yaml"
echo ""

echo "=========================================="
echo "9. Creating Instance-Specific Override"
echo "=========================================="
cat > /tmp/instance-override.json <<EOF
{
  "metadata": {
    "name": "x1000c0s0b0n0"
  },
  "spec": {
    "description": "Special configuration for node x1000c0s0b0n0",
    "instance_id": "x1000c0s0b0n0",
    "local_hostname": "special-node-1000",
    "hostname": "special-node-1000",
    "cloud_init_base_url": "${SERVER_URL}",
    "public_keys": [
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC... special-admin@node1000"
    ]
  }
}
EOF

echo "Creating instance override for x1000c0s0b0n0..."
go run $CLIENT --server "${SERVER_URL}" instanceinfo create --spec "$(cat /tmp/instance-override.json)"
echo ""

echo "Testing /meta-data with override (simulating node ${NODE0_IP}):"
curl -s -H "X-Forwarded-For: ${NODE0_IP}" "${SERVER_URL}/meta-data" | grep -A 5 "hostname"
echo ""

echo "=========================================="
echo "10. Cleanup (Optional)"
echo "=========================================="
echo ""
echo "To clean up, delete the data directory:"
echo "  rm -rf /data/*"
echo ""
echo "Or use the client to delete by UID (get UIDs from list command):"
echo "  go run $CLIENT --server ${SERVER_URL} group list"
echo "  go run $CLIENT --server ${SERVER_URL} clusterdefaults list"
echo "  go run $CLIENT --server ${SERVER_URL} instanceinfo list"
echo "  go run $CLIENT --server ${SERVER_URL} group delete <UID>"
echo ""

# Cleanup temp files
rm -f /tmp/cluster-defaults.json /tmp/compute-group.json /tmp/green-group.json \
  /tmp/blue-group.json /tmp/storage-group.json /tmp/instance-override.json

echo "=========================================="
echo "Demo Complete!"
echo "=========================================="
echo ""
echo "The API is now populated with:"
echo "  • 1 ClusterDefaults configuration"
echo "  • 4 Group templates (compute, green, blue, storage)"
echo "  • 1 InstanceInfo override"
echo ""
echo "You can continue to:"
echo "  • Test cloud-init endpoints with different node IPs"
echo "  • Create additional groups and templates"
echo "  • Update existing resources"
echo "  • Query the OpenAPI documentation at ${SERVER_URL}/openapi.json"

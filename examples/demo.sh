#!/bin/bash
# demo.sh - Demonstrates the cloud-init metadata API using the generated client
set -e

# Configuration
SERVER_URL="http://localhost:8888"
CLIENT="../cmd/client/main.go"

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
cat > /tmp/cluster-defaults.json << 'EOF'
{
    "name": "production-cluster",
    "description": "Production cluster defaults",
    "base_url": "http://localhost:8888",
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
EOF

echo "Creating cluster defaults..."
go run $CLIENT --server "${SERVER_URL}" clusterdefaults create --spec "$(cat /tmp/cluster-defaults.json)"
echo ""

echo "=========================================="
echo "2. Creating Compute Group Template"
echo "=========================================="
cat > /tmp/compute-group.json << 'EOF'
{
    "name": "compute-nodes",
    "description": "Standard compute node configuration",
    "template": "#cloud-config\nhostname: {{ hostname }}\nfqdn: {{ hostname }}.{{ cluster_name }}.local\n\n# Set timezone\ntimezone: America/Los_Angeles\n\n# Configure users\nusers:\n  - name: hpcadmin\n    groups: [sudo, docker]\n    shell: /bin/bash\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    ssh_authorized_keys:\n      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC... hpcadmin@cluster\n\n# Install required packages\npackages:\n  - vim\n  - htop\n  - iperf3\n  - numactl\n  - hwloc\n  - libibverbs-dev\n  - infiniband-diags\n  - cuda-toolkit-12-2\n\n# Mount shared filesystems\nmounts:\n  - [ \"nfsserver:/home\", \"/home\", \"nfs\", \"defaults,_netdev\", \"0\", \"0\" ]\n  - [ \"nfsserver:/opt\", \"/opt\", \"nfs\", \"defaults,_netdev\", \"0\", \"0\" ]\n\n# Configure network settings\nwrite_files:\n  - path: /etc/sysctl.d/99-hpc.conf\n    content: |\n      # HPC optimizations\n      net.core.rmem_max = 268435456\n      net.core.wmem_max = 268435456\n      net.ipv4.tcp_rmem = 4096 87380 134217728\n      net.ipv4.tcp_wmem = 4096 65536 134217728\n  - path: /etc/node-info\n    content: |\n      NODE_ID={{ instance_id }}\n      NODE_NID={{ nid }}\n      NODE_ROLE={{ role }}\n      COMPUTE_TYPE={{ compute_type }}\n\nruncmd:\n  - sysctl -p /etc/sysctl.d/99-hpc.conf\n  - systemctl enable --now slurmd\n  - echo \"Compute node {{ instance_id }} (NID: {{ nid }}) initialized\" | logger -t cloud-init\n",
    "metadata": {
      "compute_type": "gpu",
      "slurm_enabled": "true",
      "monitoring": "prometheus"
    }
}
EOF

echo "Creating compute group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/compute-group.json)"
echo ""

echo "=========================================="
echo "3. Creating Storage Group Template"
echo "=========================================="
cat > /tmp/storage-group.json << 'EOF'
{
    "name": "storage-nodes",
    "description": "Storage node configuration for Lustre/NFS servers",
    "template": "#cloud-config\nhostname: {{ hostname }}\nfqdn: {{ hostname }}.{{ cluster_name }}.local\n\nusers:\n  - name: storageadmin\n    groups: [sudo]\n    shell: /bin/bash\n    sudo: ALL=(ALL) NOPASSWD:ALL\n\npackages:\n  - lustre-client-modules\n  - lustre-utils\n  - nfs-kernel-server\n  - zfs-dkms\n  - zfsutils-linux\n\nwrite_files:\n  - path: /etc/exports\n    content: |\n      /export/home *(rw,sync,no_root_squash,no_subtree_check)\n      /export/scratch *(rw,sync,no_root_squash,no_subtree_check)\n  - path: /etc/storage-config\n    content: |\n      STORAGE_ROLE={{ storage_role }}\n      RAID_LEVEL={{ raid_level }}\n      FILESYSTEM_TYPE={{ fs_type }}\n\nruncmd:\n  - zpool create -f datapool {{ disk_list }}\n  - zfs create -o compression=lz4 datapool/export\n  - systemctl enable --now nfs-kernel-server\n  - exportfs -ra\n",
    "metadata": {
      "storage_role": "nfs-server",
      "raid_level": "raidz2",
      "fs_type": "zfs",
      "disk_list": "/dev/sdb /dev/sdc /dev/sdd /dev/sde"
    }
}
EOF

echo "Creating storage group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/storage-group.json)"
echo ""

echo "=========================================="
echo "4. Creating Login Node Group Template"
echo "=========================================="
cat > /tmp/login-group.json << 'EOF'
{
    "name": "login-nodes",
    "description": "Login node configuration with development tools",
    "template": "#cloud-config\nhostname: {{ hostname }}\nfqdn: {{ hostname }}.{{ cluster_name }}.local\n\nusers:\n  - name: hpcadmin\n    groups: [sudo]\n    shell: /bin/bash\n\npackages:\n  - build-essential\n  - gcc\n  - g++\n  - gfortran\n  - cmake\n  - git\n  - python3-dev\n  - python3-pip\n  - slurm-client\n  - environment-modules\n\nwrite_files:\n  - path: /etc/profile.d/modules.sh\n    content: |\n      # Module system initialization\n      if [ -f /usr/share/modules/init/bash ]; then\n        . /usr/share/modules/init/bash\n      fi\n  - path: /etc/motd\n    content: |\n      ╔════════════════════════════════════════╗\n      ║  {{ cluster_name }} HPC Cluster       ║\n      ║  Login Node: {{ hostname }}           ║\n      ║  Node ID: {{ instance_id }}           ║\n      ╚════════════════════════════════════════╝\n      \n      For support: support@hpc.example.com\n\nruncmd:\n  - pip3 install numpy scipy pandas matplotlib\n  - systemctl enable --now slurm-client\n",
    "metadata": {
      "node_type": "login",
      "dev_tools": "enabled"
    }
}
EOF

echo "Creating login group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/login-group.json)"
echo ""

echo "=========================================="
echo "5. Creating Specialized GPU Group Template"
echo "=========================================="
cat > /tmp/gpu-group.json << 'EOF'
{
  "name": "gpu-nodes",
    "description": "GPU-accelerated compute nodes",
    "template": "#cloud-config\nhostname: {{ hostname }}\nfqdn: {{ hostname }}.{{ cluster_name }}.local\n\npackages:\n  - nvidia-driver-535\n  - nvidia-cuda-toolkit\n  - nvidia-container-toolkit\n  - docker.io\n\nwrite_files:\n  - path: /etc/docker/daemon.json\n    content: |\n      {\n        \"runtimes\": {\n          \"nvidia\": {\n            \"path\": \"nvidia-container-runtime\",\n            \"runtimeArgs\": []\n          }\n        },\n        \"default-runtime\": \"nvidia\"\n      }\n  - path: /usr/local/bin/gpu-info.sh\n    permissions: '0755'\n    content: |\n      #!/bin/bash\n      nvidia-smi --query-gpu=index,name,driver_version,memory.total --format=csv\n  - path: /etc/slurm/gres.conf\n    content: |\n      NodeName={{ hostname }} Name=gpu Type={{ gpu_type }} File=/dev/nvidia[0-{{ gpu_count }}]\n\nruncmd:\n  - nvidia-smi\n  - docker run --rm --gpus all nvidia/cuda:12.2.0-base-ubuntu22.04 nvidia-smi\n  - systemctl restart slurmd\n",
    "metadata": {
      "gpu_type": "A100",
      "gpu_count": "7"
    }
}
EOF

echo "Creating GPU group..."
go run $CLIENT --server "${SERVER_URL}" group create --spec "$(cat /tmp/gpu-group.json)"
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

echo "Testing /meta-data endpoint (simulating node 10.0.0.100):"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/meta-data" | head -20
echo "..."
echo ""

echo "Testing /vendor-data endpoint (simulating node 10.0.0.100):"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/vendor-data"
echo ""

echo "Testing /user-data endpoint:"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/user-data"
echo ""

echo "Testing /compute-nodes.yaml endpoint (requires group membership):"
# This will fail with 404 since our mock node isn't in compute-nodes group
# but demonstrates the endpoint
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/compute-nodes.yaml" || echo "Note: Node not in compute-nodes group (expected for demo)"
echo ""

echo "=========================================="
echo "9. Creating Instance-Specific Override"
echo "=========================================="
cat > /tmp/instance-override.json << 'EOF'
{
    "name": "x1000c0s0b0n0",
     "description": "Special configuration for node x1000c0s0b0n0",
    "instance_id": "x1000c0s0b0n0",
    "local_hostname": "special-node-1000",
    "hostname": "special-node-1000",
    "cloud_init_base_url": "http://localhost:8888",
    "public_keys": [
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC... special-admin@node1000"
    ]
}
EOF

echo "Creating instance override for x1000c0s0b0n0..."
go run $CLIENT --server "${SERVER_URL}" instanceinfo create --spec "$(cat /tmp/instance-override.json)"
echo ""

echo "Testing /meta-data with override (simulating node 10.0.0.100):"
curl -s -H "X-Forwarded-For: 10.0.0.100" "${SERVER_URL}/meta-data" | grep -A 5 "hostname"
echo ""

echo "=========================================="
echo "10. Cleanup (Optional)"
echo "=========================================="
echo ""
echo "To clean up, delete the data directory:"
echo "  rm -rf ../data/*"
echo ""
echo "Or use the client to delete by UID (get UIDs from list command):"
echo "  go run $CLIENT --server ${SERVER_URL} group list"
echo "  go run $CLIENT --server ${SERVER_URL} group delete <UID>"
echo ""

# Cleanup temp files
rm -f /tmp/cluster-defaults.json /tmp/compute-group.json /tmp/storage-group.json \
      /tmp/login-group.json /tmp/gpu-group.json /tmp/instance-override.json

echo "=========================================="
echo "Demo Complete!"
echo "=========================================="
echo ""
echo "The API is now populated with:"
echo "  • 1 ClusterDefaults configuration"
echo "  • 4 Group templates (compute, storage, login, GPU)"
echo "  • 1 InstanceInfo override"
echo ""
echo "You can continue to:"
echo "  • Test cloud-init endpoints with different node IPs"
echo "  • Create additional groups and templates"
echo "  • Update existing resources"
echo "  • Query the OpenAPI documentation at ${SERVER_URL}/openapi.json"

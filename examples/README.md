# Cloud-Init Metadata API Demo

This directory contains a demonstration script that showcases the cloud-init metadata API functionality using the generated Go client.

## Prerequisites

1. **Server Running**: The metadata server must be running on port 8888
   ```bash
   cd ..
   go run ./cmd/server serve --port 8888
   ```

2. **Mock SMD Client**: The demo assumes the server is using the mock SMD client (default when `SMD_URL` is not set)

## Mock SMD Data

The demo works with the following pre-configured mock nodes:

| Component ID      | IP Address   | NID  | Role    | Groups          |
|-------------------|--------------|------|---------|-----------------|
| x1000c0s0b0n0     | 10.0.0.100   | 1000 | compute | compute, green  |
| x1000c0s0b0n1     | 10.0.0.101   | 1001 | compute | compute, blue   |
| x1000c0s1b0n0     | 10.0.0.102   | 1002 | storage | storage         |

## What the Demo Does

The `demo.sh` script demonstrates the complete lifecycle of the cloud-init metadata API:

### 1. **Creates ClusterDefaults**
   - Sets up cluster-wide configuration
   - Defines base URL, cloud provider, region
   - Configures hostname generation parameters
   - Adds cluster-wide SSH public keys

### 2. **Creates Group Templates**
   The demo creates four different group types, each with realistic configurations:

   - **Compute Nodes** (`compute-nodes`)
     - GPU-enabled compute configuration
     - HPC network optimizations
     - Slurm integration
     - Shared filesystem mounts
     - Development tools

   - **Storage Nodes** (`storage-nodes`)
     - Lustre/ZFS storage configuration
     - NFS server setup
     - RAID configuration
     - Export definitions

   - **Login Nodes** (`login-nodes`)
     - Development environment
     - Build tools and compilers
     - Python scientific stack
     - Module system setup
     - Custom MOTD

   - **GPU Nodes** (`gpu-nodes`)
     - NVIDIA driver installation
     - CUDA toolkit
     - Docker with GPU support
     - Slurm GRES configuration

### 3. **Demonstrates Template Features**
   Each template showcases different cloud-init capabilities:
   - Variable substitution (`{{ hostname }}`, `{{ instance_id }}`, `{{ nid }}`, etc.)
   - Package installation
   - File creation with dynamic content
   - User configuration
   - Network mount setup
   - Custom metadata fields

### 4. **Tests Cloud-Init Endpoints**
   - `/meta-data` - Instance metadata with merged defaults
   - `/vendor-data` - Include-file list for group configs
   - `/user-data` - Empty config (user override preservation)
   - `/{group}.yaml` - Rendered group-specific templates

### 5. **Creates Instance Overrides**
   - Demonstrates instance-specific configuration
   - Shows hostname overrides
   - Adds instance-specific SSH keys

### 6. **Validates Template Rendering**
   - Tests Jinja2 template processing
   - Verifies variable substitution
   - Confirms proper YAML formatting

## Running the Demo

```bash
# Start the server in one terminal
cd ..
go run ./cmd/server serve --port 8888

# Run the demo in another terminal
cd examples
./demo.sh
```

## Expected Output

The script will:
1. ✓ Wait for server to be ready
2. ✓ Create all resources using the generated client
3. ✓ List and retrieve resources
4. ✓ Test cloud-init endpoints with mock node IPs
5. ✓ Show rendered templates with substituted values
6. ✓ Display cleanup commands (optional)

## Example API Interactions

### Creating a Group
```bash
go run ../cmd/client/main.go --server http://localhost:8888 group create --spec "$(cat compute-group.json)"
```

### Listing Groups
```bash
go run ../cmd/client/main.go --server http://localhost:8888 group list
```

### Getting Group Details
```bash
go run ../cmd/client/main.go --server http://localhost:8888 group get compute-nodes
```

### Deleting a Group
```bash
go run ../cmd/client/main.go --server http://localhost:8888 group delete compute-nodes
```

## Testing Cloud-Init Endpoints Manually

```bash
# Get metadata for node at 10.0.0.100
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/meta-data

# Get vendor-data (include-file list)
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/vendor-data

# Get specific group configuration (requires group membership)
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/compute.yaml
```

## Template Variables

All templates have access to these variables:

### Default Variables (from cluster defaults + component info)
- `hostname` - Generated hostname (e.g., `prod001000`)
- `instance_id` - Component ID (e.g., `x1000c0s0b0n0`)
- `nid` - Node ID number (e.g., `1000`)
- `role` - Component role (e.g., `compute`)
- `cluster_name` - From ClusterDefaults

### Custom Variables (from group metadata)
Each group can define additional variables in its `metadata` field:
- `compute_type` - Type of compute node
- `gpu_type` - GPU model name
- `storage_role` - Storage node role
- `raid_level` - RAID configuration
- etc.

## Cleanup

To remove all demo resources:

```bash
go run ../cmd/client/main.go --server http://localhost:8888 group delete compute-nodes
go run ../cmd/client/main.go --server http://localhost:8888 group delete storage-nodes
go run ../cmd/client/main.go --server http://localhost:8888 group delete login-nodes
go run ../cmd/client/main.go --server http://localhost:8888 group delete gpu-nodes
go run ../cmd/client/main.go --server http://localhost:8888 clusterdefaults delete production-cluster
go run ../cmd/client/main.go --server http://localhost:8888 instanceinfo delete x1000c0s0b0n0
```

Or simply delete the data directory and restart:
```bash
rm -rf ../data/*
```

## Troubleshooting

### Server Not Running
```
Error: connection refused
Solution: Start the server with: go run ../cmd/server serve --port 8888
```

### Wrong Port
```
Error: connection refused on localhost:8888
Solution: Check that server is listening on port 8888
```

### Missing Client
```
Error: cannot find ../cmd/client/main.go
Solution: Run from the examples directory: cd examples && ./demo.sh
```

### Resource Already Exists
```
Error: resource already exists
Solution: Delete existing resource first or use a different name
```

## Advanced Usage

### Custom Node IP Testing

Test endpoints with different mock node IPs:

```bash
# Test with compute node (groups: compute, green)
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/meta-data

# Test with different compute node (groups: compute, blue)
curl -H "X-Forwarded-For: 10.0.0.101" http://localhost:8888/meta-data

# Test with storage node (groups: storage)
curl -H "X-Forwarded-For: 10.0.0.102" http://localhost:8888/meta-data
```

### Template Validation

You can validate templates before creating them by checking the response:

```bash
# Create group and check validation
go run ../cmd/client/main.go --server http://localhost:8888 group create --spec "$(cat my-template.json)"

# If validation fails, you'll see error messages about missing variables
```

### Updating Templates

To update an existing group template:

```bash
# Get current version
go run ../cmd/client/main.go --server http://localhost:8888 group get compute-nodes --output json > current.json

# Edit current.json with your changes

# Update the group
go run ../cmd/client/main.go --server http://localhost:8888 group update compute-nodes --spec "$(cat current.json)"
```

## Real-World Usage

In production, you would:

1. **Configure Real SMD**: Set `SMD_URL` environment variable to point to your State Management Database
2. **Create Groups**: Define groups matching your cluster organization
3. **Assign Nodes**: Use SMD to assign nodes to appropriate groups
4. **Deploy Configs**: Nodes fetch their configuration via cloud-init on boot
5. **Update Templates**: Modify group templates and nodes pick up changes on next boot

## See Also

- [Main README](../README.md) - Project overview
- [CLOUDINIT.md](../CLOUDINIT.md) - Cloud-init endpoint documentation
- [Generated Client](../cmd/client/) - CLI client source code
- [API Server](../cmd/server/) - Server source code

# Cloud-Init Metadata Service - AI Coding Agent Instructions

## Project Overview

This is a cloud-init metadata server built using the OpenCHAMI Fabrica framework. It provides cloud-init datasource endpoints for High Performance Computing (HPC) nodes, with template-based configuration management and integration with State Management Database (SMD).

### Replacement for Legacy Service

This service replaces the original [OpenCHAMI/cloud-init](https://github.com/OpenCHAMI/cloud-init) implementation with a Fabrica-based architecture. Key differences:

**Legacy Service** (v1.4.0):
- Direct HTTP handlers with custom routing
- Server-side config merging (complexity in server)
- Group data stored as JSON with Jinja templates in API payloads
- Base64-encoded content in POST requests
- Impersonation mode for testing (`/cloud-init/admin/impersonation/{id}/meta-data`)
- Manual SMD simulator mode (`CLOUD_INIT_SMD_SIMULATOR=true`)
- Runs on port 27777 by default

**This Service** (Fabrica-based):
- Auto-generated REST API from resource definitions
- Client-side config merging (pushed to cloud-init client)
- Resources (Group, ClusterDefaults, InstanceInfo) with validation
- Template rendering validated server-side with Pongo2
- Mock SMD client for development (no impersonation routes needed)
- Runs on port 8080/8888 by default
- Built-in versioning, validation, and storage abstraction

## Architecture & Key Components

### Core Technologies
- **Fabrica Framework**: Auto-generates REST API, storage, and client from resource definitions in `pkg/resources/`
- **Pongo2 Templates**: Jinja2-compatible templating for cloud-init configs with variable substitution
- **SMD Integration**: Hardware component lookup and group membership via `pkg/smdclient/` interface
- **Chi Router**: HTTP routing with cloud-init specific endpoints in `cmd/server/cloudinit_routes.go`

### Resource Model (3 Core Types)
```
pkg/resources/
├── group/          # Template-based node group configs with Jinja2 rendering
├── clusterdefaults/# Cluster-wide defaults (hostnames, SSH keys, regions)
└── instanceinfo/   # Instance-specific overrides (per-node customization)
```

### Generated Code Structure
- `cmd/server/*_handlers_generated.go` - REST API handlers (DO NOT EDIT)
- `cmd/server/models_generated.go` - Request/response models (DO NOT EDIT)
- `internal/storage/storage_generated.go` - File-based storage operations (DO NOT EDIT)

### Cloud-Init Integration
- **Identity Resolution**: IP-based via SMD lookup (`X-Forwarded-For` header support)
- **Endpoints**: `/meta-data`, `/user-data`, `/vendor-data`, `/{group}.yaml`
- **Template Variables**: Runtime injection from ClusterDefaults + SMD Component data
- **Group Membership**: SMD-based authorization for group-specific configs

## Critical Development Patterns

### Resource Validation
Custom validation in `pkg/resources/group/group.go`:
```go
func (r *Group) Validate(ctx context.Context) error {
    // Extract template variables using regex
    vars := extractTemplateVariables(r.Spec.Template)
    // Merge runtime data for validation
    merged := MergeMetadata(sampleMetadata(), r.Spec.MetaData)
    // Template rendering validation with pongo2
    rendered, err := RenderTemplate(r.Spec.Template, merged)
    // YAML syntax validation of rendered output
    return validateYAML(rendered)
}
```

### Template Variable System
Runtime variables automatically injected during validation and rendering:
- **From ClusterDefaults**: `cluster_name`, `base_url`, `cloud_provider`, `region`
- **From SMD Component**: `hostname`, `instance_id`, `nid`, `role`, `mac`, `ip`
- **Custom Variables**: User-defined in `Group.Spec.MetaData`

### SMD Client Pattern
Interface-based design for testing (`pkg/smdclient/`):
```go
// Real implementation connects to SMD HTTP API
// Mock implementation provides 3 test nodes for development
if smdURL := os.Getenv("SMD_URL"); smdURL != "" {
    // Production: HTTP client
} else {
    // Development: Mock with x1000c0s0b0n0-2 nodes
}
```

## Development Workflow

### Code Generation
```bash
fabrica generate  # Regenerate handlers, storage, client from resource definitions
```

### Local Development
```bash
# Start server (uses mock SMD by default)
go run ./cmd/server serve --port 8888

# Run comprehensive demo
cd examples && ./demo.sh

# Test cloud-init endpoints manually
curl -H "X-Forwarded-For: 10.0.0.100" http://localhost:8888/meta-data
```

### Testing Approach
- **Unit Tests**: Focus on custom validation logic in `pkg/resources/*/`
- **Integration Tests**: Cloud-init handlers in `pkg/handlers/metadata_test.go`
- **Demo Script**: End-to-end workflow validation in `examples/demo.sh`

## Key Configuration Files

- `.fabrica.yaml` - Framework configuration (validation, storage type, features)
- `CLOUDINIT.md` - Cloud-init endpoint documentation
- `examples/README.md` - Demo usage and API examples

## Common Patterns & Gotchas

### Storage Backend
File-based storage in `./data/{resource-type}/` with JSON serialization. Status fields persist automatically when validation modifies them.

### Template History Tracking
Version tracking implemented in custom `Validate()` method using SHA256 hashes:
```go
func (r *Group) trackTemplateVersion(valid bool, errorMsg string) {
    version := generateTemplateVersion(r.Spec.Template) // v-[8char-hash]
    // Append to r.Status.TemplateHistory (last 10 versions)
}
```

### Client Usage
Generated client requires specific flags:
```bash
go run ./cmd/client/main.go --server http://localhost:8888 group create --spec "$(cat group.json)"
# Note: --spec (not --data), resource retrieval by UID (not name)
```

### Adding New Resources
1. Create struct in `pkg/resources/newtype/` 
2. Implement custom `Validate()` if needed
3. Run `fabrica generate`
4. Handlers, storage, client auto-generated

## Production Considerations

- Set `SMD_URL` environment variable for real SMD integration
- Configure `--data-dir` for persistent storage location  
- Template validation runs on every create/update operation
- Group membership authorization enforced via SMD for `/{group}.yaml` endpoints

## Migration from Legacy Service

### Endpoint Compatibility
Both services implement the nocloud-net datasource standard endpoints:
- `/meta-data` - Instance metadata (compatible)
- `/user-data` - User-provided config (compatible, returns empty `#cloud-config`)
- `/vendor-data` - Include-file list pointing to group configs (compatible)
- `/{group}.yaml` - Group-specific templates (compatible, requires group membership)

### Configuration Migration Pattern
Legacy service stored group data via POST to `/cloud-init/admin/groups/`:
```bash
# Legacy approach
curl -X POST http://localhost:27777/cloud-init/admin/groups/ \
  -H "Content-Type: application/json" \
  -d '{"name": "compute", "data": {...}, "file": {"content": "...", "encoding": "plain"}}'
```

New service uses generated client with resource model:
```bash
# Fabrica approach
go run ./cmd/client/main.go --server http://localhost:8888 group create \
  --spec '{"name": "compute", "template": "...", "metadata": {...}}'
```

### Development Mode Differences
- **Legacy**: Set `CLOUD_INIT_SMD_SIMULATOR=true` + use impersonation routes
- **New**: Mock SMD client auto-enabled when `SMD_URL` not set, test with `X-Forwarded-For` header

### Breaking Changes
- No `/cloud-init/admin/` prefix on management endpoints
- Group templates must be valid Jinja2 + produce valid YAML (validated on create)
- Template history tracking built-in (SHA256 versioning)
- Resources identified by UID, not just name

## Compatibility Analysis with Legacy Service

### ✅ Compatible Cloud-Init Endpoints
Both services produce identical nocloud-net datasource responses:

**`/meta-data`**: Returns YAML with `instance-id`, `local-hostname`, `hostname`, `cluster-name`, and nested `instance_data.v1` structure including `vendor_data` with groups metadata.

**`/user-data`**: Returns `#cloud-config\n` (intentional no-op for user overrides).

**`/vendor-data`**: Returns `#include` format list of group YAML URLs.

**`/{group}.yaml`**: Returns rendered group template with SMD-based authorization.

### ⚠️ Potential Incompatibilities

**1. Metadata Structure Differences**
- **Legacy**: `instance_data.v1.local_ipv4` can be `interface{}` (string or complex type)
- **New**: Always string type for `local_ipv4`
- **Impact**: Clients expecting complex IP structures may break

**2. Group Metadata in vendor_data**
- **Legacy**: Groups nested under `instance_data.v1.vendor_data.groups[name]` with all custom key/values merged directly
- **New**: Same structure but metadata sourced from `Group.Spec.MetaData` instead of `GroupData.Data`
- **Impact**: Group data migration required - old `data` field → new `metadata` field

**3. Template Storage Format**
- **Legacy**: Group templates stored with `file.content` and `file.encoding` (supports base64)
- **New**: Templates stored in `Group.Spec.Template` (plain text only, validated on create)
- **Impact**: Must decode base64 templates during migration

**4. Admin API Endpoints**
- **Legacy**: `/admin/groups/`, `/admin/cluster-defaults`, `/admin/instance-info/{id}`
- **New**: `/groups`, `/clusterdefaults`, `/instanceinfo` (resource-style endpoints)
- **Impact**: Admin scripts/tooling must update URLs and request formats

**5. Impersonation/Testing**
- **Legacy**: Explicit impersonation routes (`/admin/impersonation/{id}/meta-data`)
- **New**: Test with `X-Forwarded-For` header + mock SMD client
- **Impact**: Different testing workflow for development

**6. Group Template Validation**
- **Legacy**: No server-side validation; templates can fail at render time
- **New**: Templates validated on create/update (must render with sample data + produce valid YAML)
- **Impact**: Invalid templates rejected at creation time (prevents runtime failures but requires fix-before-deploy)

**7. Hostname Generation Edge Cases**
- **Legacy**: Uses `fmt.Sprintf("%.2s", clusterName)` for short names (truncates to 2 chars)
- **New**: Uses `clusterName[:2]` if length >= 2, else full `clusterName`
- **Impact**: Single-character cluster names handled differently (legacy: blank prefix, new: single char)

**8. Missing nid/role/mac in Group Template Context**
- **Legacy**: Group templates have access to `vendor_data.nid`, `vendor_data.role` from meta-data merge
- **New**: Template context includes `nid`, `role` directly but NOT `mac` or `ip` 
- **Impact**: Templates using `{{ mac }}` or `{{ ip }}` will fail validation (need to add these to template context)

### 🔧 Migration Checklist
1. **Decode base64 templates**: Convert `file.encoding=base64` to plain text
2. **Update group data structure**: Rename `data` → `metadata` in group definitions
3. **Add hostname variables**: Ensure templates use `{{ hostname }}` not hostname from meta-data
4. **Test template validation**: Fix templates that don't render with sample metadata
5. **Update admin tooling**: Change API endpoint URLs and request formats
6. **Add missing template variables**: Include `mac`, `ip` in group template rendering context
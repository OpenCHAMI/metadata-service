<!--
SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# github.com/OpenCHAMI/cloud-init



## Getting Started

1. Define your resources in pkg/resources/
2. Generate code: fabrica generate
3. Run the server: go run cmd/server/main.go

## Configuration

The server supports configuration via:
- Command line flags
- Environment variables (GITHUB.COM/OPENCHAMI/CLOUD-INIT_*)
- Configuration file (~/.github.com/OpenCHAMI/cloud-init.yaml)

## Features

- 💾 File-based storage

## Development

```bash
# Install dependencies
go mod tidy

# Run the server
go run cmd/server/main.go serve

# Run with custom config
go run cmd/server/main.go serve --config config.yaml
```

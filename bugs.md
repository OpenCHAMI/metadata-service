<!--
SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# metadata-service — service binary bugs

This file tracks bugs in the service binary and image behavior that operators currently work around, but should be fixed upstream.

## 1) `--data-dir` and `--wireguard-state-file` style flags can be silently ignored

- Where: [cmd/server/main.go](cmd/server/main.go#L37) has underscore-based tags (`data_dir`, `wireguard_state_file`), while flags are dash-based (`--data-dir`, `--wireguard-state-file`) at [cmd/server/main.go](cmd/server/main.go#L105) and [cmd/server/main.go](cmd/server/main.go#L108).
- Details: `viper.BindPFlags` stores dash keys; `viper.Unmarshal` reads underscore keys from struct tags. Without aliases, flag and env values can be missed.
- Current workaround: Operator avoids relying on affected flags/env mappings.
- Proposed fix: add aliases for every dash-bearing flag in `init()` after `viper.BindPFlags`, or use the generic alias loop in `initConfig()` after `viper.AutomaticEnv()`:

```go
for _, key := range viper.AllKeys() {
	if strings.Contains(key, "-") {
		viper.RegisterAlias(strings.ReplaceAll(key, "-", "_"), key)
	}
}
```

## 2) Docker default `./data` is fragile with restricted PSP

- Where: [Dockerfile](Dockerfile#L7) sets `WORKDIR /app`, and default data path is relative (`./data`).
- Details: With read-only root filesystem policies, `/app/data` is often unwritable unless callers mount writable storage at that exact path.
- Current workaround: Operator mounts writable `emptyDir` at `/app/data`.
- Proposed fix:
- Make `--data-dir` overrides reliable via alias fix above.
- Consider changing the default to an absolute writable path (for example `/data`) and document mount expectations.

## 3) `/meta-data` can fail with `node not found` when SMD IP lookup returns an empty JSON body

- Where: [pkg/handlers/metadata.go](pkg/handlers/metadata.go#L107) resolves the caller IP through `smd.IDfromIP()`, and [pkg/smdclient/http_client.go](pkg/smdclient/http_client.go#L52) unmarshals the `/Inventory/EthernetInterfaces` response.
- Details: In the reported failure, the metadata request logs `unexpected end of JSON input` while resolving the node ID from the request IP. That means the `/meta-data` handler never reaches metadata rendering; it short-circuits to `404 node not found` when the SMD lookup response is empty or not valid JSON. There is also no visible reverse lookup from a WireGuard tunnel IP back to the component ID in the request path, so if the request is arriving from the VPN address rather than the node's physical IP, the code may be asking SMD to resolve an address it never stores.
- Current workaround: Use a mock SMD client or verify that the configured SMD endpoint returns a JSON list for the IP lookup path before booting nodes. If the request is coming over WireGuard, confirm the tunnel IP is mapped to a component ID somewhere before the metadata handler runs.
- Proposed fix: Add an explicit WireGuard IP -> component ID lookup before `IDfromIP()`, or persist and query the tunnel address mapping alongside the existing `AddWGIP()` flow. Harden `IDfromIP()` against empty/204 responses and add a clearer error path for lookup failures, or switch the lookup to the SMD endpoint/parameter that reliably returns the component ID for a client IP.

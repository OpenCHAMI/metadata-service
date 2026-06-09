# Testing Guide: Reconciliation Runtime & WireGuard Peer Lifecycle

## Prerequisites

Two terminal sessions — one for the server, one for testing. You'll also need `jq` and `wg` (WireGuard tools) installed.

---

### 1. Start the server with WireGuard enabled

```bash
go run ./cmd/server/ serve \
  --data-dir /tmp/metadata-test \
  --wireguard-server 100.97.0.1/24 \
  --wireguard-state-file /tmp/metadata-test/wg-state.yaml
```

Expected startup log lines:
- `"WireGuard userspace controller enabled on 100.97.0.1/24"`
- `"Mock SMD client initialized with sample data"`
- `"Reconciliation runtime initialized"`

---

### 2. Test WireGuard peer initialization (`POST /wg-init`)

```bash
PUBKEY=$(wg genkey | wg pubkey)

curl -s -X POST http://localhost:8080/wg-init \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$PUBKEY\"}" | jq .
```

Expected response (HTTP 202):
```json
{
  "message": "WireGuard peer allocation accepted",
  "peer-uid": "wireguardpeer-<hex>",
  "client-vpn-ip": "100.97.0.2",
  "server-public-key": "<base64>",
  "server-ip": "100.97.0.1",
  "server-port": "51820"
}
```

---

### 3. Test idempotency (same key returns same config)

```bash
curl -s -X POST http://localhost:8080/wg-init \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$PUBKEY\"}" | jq .
```

Expected: Same `peer-uid` and `client-vpn-ip` as the first call.

---

### 4. Verify the peer via the REST API

```bash
curl -s http://localhost:8080/wireguardpeers | jq .
curl -s http://localhost:8080/wireguardpeers/wireguardpeer-<hex> | jq .
```

Expected: Status shows `"phase": "Ready"` if reconciliation succeeded. `"Degraded"` is also normal when running outside the reconciler's event bus (peer still works).

---

### 5. Test phone-home de-registration (`POST /phone-home/{id}`)

```bash
curl -s -X POST http://localhost:8080/phone-home/wireguardpeer-<hex> -w "\nHTTP %{http_code}\n"
curl -s http://localhost:8080/wireguardpeers/wireguardpeer-<hex>
```

Expected: HTTP 200, then 404 (peer deleted).

---

### 6. Test REST API CRUD directly

```bash
curl -s -X POST http://localhost:8080/wireguardpeers \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "v1",
    "kind": "WireGuardPeer",
    "metadata": {"name": "test-peer"},
    "spec": {
      "publicKey": "'"$(wg genkey | wg pubkey)"'",
      "allowedIP": "100.97.0.42/32"
    }
  }' | jq .

curl -s http://localhost:8080/wireguardpeers | jq '.items | length'

PEER_UID=$(curl -s http://localhost:8080/wireguardpeers | jq -r '.items[0].metadata.uid')
curl -s -X DELETE "http://localhost:8080/wireguardpeers/$PEER_UID" -w "HTTP %{http_code}\n"
```

---

### 7. Test WireGuard-only access restriction

```bash
go run ./cmd/server/ serve \
  --data-dir /tmp/metadata-test-wgonly \
  --wireguard-server 100.97.0.1/24 \
  --wireguard-only \
  --port 8081

curl -s http://localhost:8081/wg-init -w "\nHTTP %{http_code}\n"                                    # 403
curl -s -H "X-Forwarded-For: 100.97.0.5" http://localhost:8081/wg-init \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$(wg genkey | wg pubkey)\"}" | jq .                                         # 202
```

---

### 8. Run the test suite

```bash
go test ./cmd/server/... ./pkg/reconcilers/... ./pkg/wireguard/... -v -count=1 2>&1 | \
  grep -E "^(=== RUN|--- PASS|--- FAIL|ok |FAIL)"
```

---

### 9. Test state persistence

Stop the server from step 1 (Ctrl+C), then restart:

```bash
go run ./cmd/server/ serve \
  --data-dir /tmp/metadata-test \
  --wireguard-server 100.97.0.1/24 \
  --wireguard-state-file /tmp/metadata-test/wg-state.yaml

curl -s http://localhost:8080/wireguardpeers | jq '.items | length'
```

Expected: Previous peers are still present (reconciled from storage on startup).

---

### Cleanup

```bash
rm -rf /tmp/metadata-test /tmp/metadata-test-wgonly
```

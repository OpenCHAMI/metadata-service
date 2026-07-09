# Metadata Service Load Tests

Performance and scalability tests for metadata-service using k6.

## Prerequisites

### Install k6

**macOS:**
```bash
brew install k6
```

**Linux:**
```bash
# Debian/Ubuntu
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6
```

**Verify:**
```bash
k6 version
```

## Test Scenarios

### 1. Smoke Test (`smoke.js`)
Quick validation with 10 VUs for 30 seconds.
```bash
k6 run load-tests/smoke.js
```

### 2. Staged Boot (`staged-boot.js`)
Simulates rolling boot: 1K → 2K → 5K → 10K nodes.
```bash
k6 run load-tests/staged-boot.js
```

### 3. Cold Boot Storm (`boot-storm.js`)
Worst-case: 10K nodes boot simultaneously.
```bash
k6 run load-tests/boot-storm.js
```

### 4. Endurance Test (`endurance.js`)
Sustained 2K concurrent for 10 minutes.
```bash
k6 run load-tests/endurance.js
```

## Configuration

Set environment variables to override defaults:

```bash
# Target metadata-service endpoint
export METADATA_SERVICE_URL="http://localhost:8080"

# Enable mock SMD mode (bypasses real SMD)
export MOCK_SMD=true

# Number of unique nodes to simulate
export NUM_NODES=10000
```

## Running Against Local Service

### Option 1: Direct Local Run
```bash
# Terminal 1: Start metadata-service
cd ~/Development/OpenCHAMI/metadata-service
make build
./server --mock-smd --port 8080

# Terminal 2: Run load test
k6 run load-tests/smoke.js
```

### Option 2: Docker Compose
```bash
docker-compose -f load-tests/docker-compose.yml up
```

## Monitoring During Tests

### Enable pprof
```bash
# Start service with pprof enabled (already included in cmd/server/main.go)
./server --mock-smd --debug

# In another terminal during load test:
curl http://localhost:6060/debug/pprof/mutex > mutex.prof
go tool pprof -http=:8081 mutex.prof
```

### Watch Metrics
```bash
# Goroutines
watch -n 1 'curl -s http://localhost:6060/debug/pprof/goroutine?debug=1 | grep "goroutine profile:" '

# Memory
watch -n 1 'ps aux | grep metadata-service'
```

## Success Criteria

| Test | P99 Target | Error Rate | Notes |
|------|-----------|------------|-------|
| Smoke | < 500ms | 0% | Baseline |
| Staged Boot | < 1s | < 1% | Realistic scenario |
| Boot Storm | < 2s | < 1% | Worst case |
| Endurance | < 1s | < 0.1% | Memory leak detection |

## Expected Failures (Pre-Fix)

Before implementing async WireGuard persistence, expect:
- Boot Storm (10K): **30-50% failure rate**, P99 > 10s
- Staged Boot (5K): ~10-20% failure rate
- Smoke (10): Should pass

## Interpreting Results

### Good Output
```
✓ status is 200
✓ response time < 2s
http_req_duration..............: avg=450ms  p(95)=800ms p(99)=1.2s
http_req_failed................: 0.05%  // <1% failures
```

### Bad Output (Lock Contention)
```
✗ response time < 2s
http_req_duration..............: avg=8.5s  p(95)=15s p(99)=timeout
http_req_failed................: 35.2%  // High failure rate
```

Check mutex profile:
```bash
go tool pprof http://localhost:6060/debug/pprof/mutex
> top10
# Should show high contention on wireguard.Controller.PeersMutex
```

## Troubleshooting

### Connection Refused
```
ERRO[0001] GoError: Get "http://localhost:8080/meta-data": dial tcp: connection refused
```
**Fix:** Ensure metadata-service is running on port 8080.

### File Descriptor Limit
```
ERRO[0030] too many open files
```
**Fix:** Increase ulimit
```bash
ulimit -n 10000
```

### k6 Installation Failed
**macOS:** Ensure Homebrew is updated: `brew update`  
**Linux:** Follow official docs: https://k6.io/docs/getting-started/installation/

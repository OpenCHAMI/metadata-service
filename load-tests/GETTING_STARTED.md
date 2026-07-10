<!--
SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors

SPDX-License-Identifier: MIT
-->

# Load Testing - Getting Started

## Quick Start

```bash
# Terminal 1: Start metadata-service
cd ~/Development/OpenCHAMI/metadata-service
make build
./server --mock-smd --port 8080 --debug

# Terminal 2: Run load tests
cd ~/Development/OpenCHAMI/metadata-service
./load-tests/quick-start.sh
```

## What to Expect

### Smoke Test (10 VUs, 30s)
**Expected:** ✅ **PASS**
- P99 < 500ms
- 0% failure rate
- Validates basic functionality

### Staged Boot (1K→10K, 5min)
**Expected:** ✅ **PASS**
- All waves (1K-10K): Fast and smooth
- P99 < 1s across all stages
- <1% failure rate

### Cold Boot Storm (10K concurrent, 3min)
**Expected:** ✅ **PASS**
- P99 < 5s (worst-case storm scenario)
- <1% failure rate
- **Fixed:** Async WireGuard persistence eliminates lock contention

**How the fix works:**
```go
// pkg/wireguard/controller.go
func (c *Controller) AddPeer(...) error {
    c.PeersMutex.Lock()
    defer c.PeersMutex.Unlock()

    // ... add peer ...

    c.enqueuePersistUnlocked()  // ← Non-blocking snapshot enqueue
}

// Background worker handles disk I/O outside the lock
func (c *Controller) persistWorker() {
    for state := range c.persistQueue {
        c.Persistence.Save(state)
    }
}
```

## Individual Test Commands

```bash
# Smoke test
make -C load-tests smoke

# Staged boot
make -C load-tests staged

# Boot storm
make -C load-tests storm

# Endurance test (10 minutes)
make -C load-tests endurance

# All tests
make -C load-tests all
```

## Monitoring During Tests

### Live Metrics
```bash
# Goroutines (watch for leaks)
watch -n 1 'curl -s http://localhost:6060/debug/pprof/goroutine?debug=1 | head -1'

# Memory (watch for growth)
watch -n 1 'ps aux | grep "[m]etadata-service"'

# Request rate (from k6 output)
# Look for: "http_reqs..............: X req/s"
```

### Capture Profiles
```bash
# During peak load (wait 30s after starting test):
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
curl http://localhost:6060/debug/pprof/mutex > mutex.prof
curl http://localhost:6060/debug/pprof/heap > heap.prof

# Analyze
go tool pprof -http=:8081 mutex.prof
```

## Interpreting Results

### Good Output (Expected)
```
✓ status is 200
✓ response time < 5s
http_req_duration..............: avg=450ms  p(95)=800ms p(99)=1.2s
http_req_failed................: 0.05%  // <1% failures
```

### Investigate If You See
```
⚠️  response time approaching threshold
http_req_duration..............: avg=2.5s  p(95)=4s p(99)=8s
http_req_failed................: 0.5%  // Some failures
```

If latencies are consistently high, capture and analyze profiles.

## Success Criteria

| Test | P99 Target | Error Rate | Status |
|------|-----------|------------|--------|
| Smoke | < 500ms | 0% | ✅ Should pass |
| Staged | < 1s | < 1% | ✅ Should pass |
| Storm | < 5s | < 1% | ✅ Should pass |
| Endurance | < 1s | < 0.1% | ✅ Should pass |

## Files Created

- `load-tests/README.md` - Full documentation
- `load-tests/common.js` - Shared utilities
- `load-tests/smoke.js` - Quick validation (10 VUs)
- `load-tests/staged-boot.js` - Rolling boot (1K→10K)
- `load-tests/boot-storm.js` - Worst case (10K concurrent)
- `load-tests/endurance.js` - Leak detection (10 min)
- `load-tests/Makefile` - Convenience targets
- `load-tests/quick-start.sh` - Interactive runner
- `load-tests/.gitignore` - Ignore results/profiles

All staged and ready to commit!

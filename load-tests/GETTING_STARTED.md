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

## What to Expect (Pre-Fix)

### Smoke Test (10 VUs, 30s)
**Expected:** ✅ **PASS**
- P99 < 500ms
- 0% failure rate
- Validates basic functionality

### Staged Boot (1K→10K, 5min)
**Expected:** ⚠️ **PARTIAL PASS**
- 1K-2K waves: Fast (~200ms P99)
- 5K wave: Starting to slow (~1s P99)
- 10K wave: ~5-10% failure rate

### Cold Boot Storm (10K concurrent, 3min)
**Expected:** ❌ **FAIL** (This is intentional!)
- P99 > 5s (likely 10-15s)
- **30-50% failure rate**
- Lock contention on `wireguard.Controller.PeersMutex`

**Why it fails:**
```go
// pkg/wireguard/controller.go
func (c *Controller) AddPeer(...) error {
    c.PeersMutex.Lock()
    defer c.PeersMutex.Unlock()

    // ... add peer ...

    return c.persistState()  // ← 10ms disk I/O while holding lock
}
```

## Test-Driven Development Flow

1. **Establish Baseline** (this step)
   ```bash
   ./load-tests/quick-start.sh
   # Save results: load-tests/results/*.json
   ```

2. **Capture Mutex Profile**
   ```bash
   curl http://localhost:6060/debug/pprof/mutex > baseline-mutex.prof
   go tool pprof -http=:8081 baseline-mutex.prof
   # Look for: wireguard.Controller.PeersMutex contention
   ```

3. **Implement Fix** (async persistence)
   - Move `persistState()` to background goroutine
   - Use buffered channel as queue
   - Reduce lock hold time: 10ms → <100µs

4. **Re-run Tests**
   ```bash
   make build
   ./server --mock-smd --port 8080 --debug
   ./load-tests/quick-start.sh
   ```

5. **Verify Improvement**
   - Boot storm failure rate: 50% → <1%
   - P99 latency: 10s → <2s
   - Mutex profile: 100x reduction in contention

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

### Good Output (Post-Fix)
```
✓ status is 200
✓ response time < 2s
http_req_duration..............: avg=450ms  p(95)=800ms p(99)=1.2s
http_req_failed................: 0.05%  // <1% failures
```

### Bad Output (Pre-Fix)
```
✗ response time < 2s
http_req_duration..............: avg=8.5s  p(95)=15s p(99)=timeout
http_req_failed................: 35.2%  // High failure rate
```

## Success Criteria

| Test | P99 Target | Error Rate | Status |
|------|-----------|------------|--------|
| Smoke | < 500ms | 0% | Should pass now |
| Staged | < 1s | < 1% | Will fail at 10K wave |
| Storm | < 2s | < 1% | Will fail (30-50% errors) |
| Endurance | < 1s | < 0.1% | May pass with leaks |

## Next Steps After Failure

1. **Confirm the bottleneck** with mutex profiling
2. **Implement async persistence** (see load test strategy doc)
3. **Re-run tests** to measure improvement
4. **Document results** in vault

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

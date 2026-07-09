# Load Testing Setup - Ready to Run

## What We Built

✅ **4 k6 Load Test Scenarios:**
- `smoke.js` - 10 VUs × 30s (quick validation)
- `staged-boot.js` - 1K→2K→5K→10K ramping (realistic)
- `boot-storm.js` - 10K concurrent (worst-case storm)
- `endurance.js` - 2K sustained × 10min (leak detection)

✅ **Mock SMD Populated:**
- 10,000 nodes pre-loaded in `createMockSMDClient()`
- Node IDs: `x1000c0s0b0n0` through `x1009c9s9b0n9`
- IPs: `10.1.0.0` through `10.1.39.15`
- **Exactly matches** k6 test generation logic

✅ **Makefile Fixed:**
- Removed `load-tests/` prefix from paths
- Targets: `make smoke`, `make storm`, etc.

## Quick Start

```bash
# Terminal 1: Start metadata-service with mock SMD
cd ~/Development/OpenCHAMI/metadata-service
./bin/ochami-metadata-server --mock-smd --port 8080 --debug

# Terminal 2: Verify mock SMD populated
cd ~/Development/OpenCHAMI/metadata-service
./load-tests/verify-mock-smd.sh

# Terminal 3: Run load tests
cd ~/Development/OpenCHAMI/metadata-service/load-tests
make smoke     # Should pass
make staged    # Will fail at 10K wave
make storm     # Will fail hard (expected!)
```

## Expected Results (Pre-Fix)

### Smoke Test
**Status:** ✅ **PASS**
- 10 VUs hitting same 10 nodes repeatedly
- P99 < 500ms
- 0% failure

### Staged Boot
**Status:** ⚠️ **PARTIAL PASS**
- 1K-2K: Fast
- 5K: Slowing
- 10K: ~5-10% failure rate
- **Bottleneck:** WireGuard lock contention starts appearing

### Boot Storm (The Killer)
**Status:** ❌ **FAIL** (This is the test!)
- **Expected:** 30-50% failure rate
- P99 > 10s (likely timeout)
- **Reason:** `wireguard.Controller.PeersMutex` locked for ~10ms per AddPeer

## Files Modified

```
M  cmd/server/smd.go                    # Call populateMockSMDForLoadTest()
A  cmd/server/smd_loadtest.go           # 10K node generation
M  load-tests/Makefile                  # Fixed paths
A  load-tests/*.js                      # All test scenarios
A  load-tests/verify-mock-smd.sh        # Validation script
```

## Commit and Run

```bash
git add -A
git commit -F COMMIT_MSG.txt
./load-tests/quick-start.sh
```

## Next Steps After Seeing Failures

1. **Capture mutex profile:**
   ```bash
   curl http://localhost:6060/debug/pprof/mutex > baseline-mutex.prof
   go tool pprof -http=:8081 baseline-mutex.prof
   ```

2. **Implement async persistence fix** (in `pkg/wireguard/controller.go`)

3. **Re-run tests and verify <1% failure rate**

---

**Status:** ✅ Ready for test-driven development  
**Branch:** feature/load-testing  
**Build:** Passing  
**Mock SMD:** 10,000 nodes loaded

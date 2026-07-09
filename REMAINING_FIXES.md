# Remaining Optimizations for 10K Scale

Based on the load test strategy analysis, here are the remaining potential bottlenecks and fixes:

## Status Summary

| Issue | Priority | Status | Impact at 10K |
|-------|----------|--------|---------------|
| WireGuard Lock + Disk I/O | 🔴 CRITICAL | ✅ **FIXED** | Was 30-50% failure |
| File Storage Backend | 🟡 HIGH | ⏳ Investigating | Unknown |
| SMD Client Timeout | 🟡 MEDIUM | 📋 Planned | Low (cache hit >90%) |
| HTTP ReadHeaderTimeout | 🟢 LOW | 📋 Planned | Slowloris protection |

---

## ✅ COMPLETED

### 1. WireGuard Controller Async Persistence
**Status:** ✅ Implemented  
**File:** `pkg/wireguard/controller.go`  
**Impact:** 100x reduction in lock hold time (10ms → <100µs)  
**Expected Result:** Failure rate 30-50% → <1%

---

## ✅ INVESTIGATION COMPLETE

### 2. File Storage Backend Concurrency Analysis
**Status:** ✅ **Confirmed Bottleneck**  
**Priority:** 🔴 **HIGH** (if write-heavy workload)  
**Finding:** Fabrica FileBackend uses **global `sync.RWMutex`**

**Analysis Results:**
- **Lock Granularity:** GLOBAL (per FileBackend instance)
- **Read Pattern:** Parallel via `RLock()` (multiple concurrent reads allowed)
- **Write Pattern:** Serialized via `Lock()` (blocks ALL reads and writes)
- **Documentation:** Explicitly states "Not optimized for large numbers of resources" and "suitable for development and testing"

**Bottleneck Impact at 10K Concurrent:**
1. ✅ **Reads are parallel** - 10K concurrent `Load()` operations can execute simultaneously
2. ❌ **Single write blocks everything** - Any `Save()` or `Delete()` blocks all 10K requests
3. ❌ **Write bursts will cause stalls** - Boot storm with writes will serialize completely

**Workload Analysis Needed:**
- **Question:** What % of requests are writes during boot?
- **Typical Boot Flow:**
  - `/meta-data` → Read (ClusterDefaults, InstanceInfo)
  - `/user-data` → Read (Group templates)
  - `/vendor-data` → Read (ClusterDefaults)
  - **Likely: >95% reads, <5% writes**

**Decision Tree:**
```
IF read:write ratio > 95:5
  → ✅ Acceptable - global RLock is fine for parallel reads
  → Monitor write latency during tests
ELSE IF writes cause >5% slowdown
  → Switch to PostgreSQL backend (Option B below)
```

**Mitigation Options:**

**Option A: Accept as-is** (Recommended for now)
- **When:** If workload is >95% reads
- **Effort:** 0 hours
- **Verification:** Run load tests, check write impact
- **Risk:** Write bursts during boot will serialize

**Option B: Switch to PostgreSQL + Ent Backend** (If needed)
- **When:** If writes cause >5% latency degradation
- **Effort:** 4-8 hours
- **Benefits:** Row-level locking, professional concurrency
- **Steps:**
  1. Add PostgreSQL to docker-compose
  2. Update `cmd/server/main.go`: `storage.InitEntBackend(dbURL)`
  3. Add `--db-url` flag
  4. Migrate data (if production)
  5. Re-run load tests

**Option C: Shard FileBackend** (Not recommended)
- **When:** Never - too much effort for little gain
- **Effort:** 40+ hours (rewrite FileBackend)
- **Why not:** Just use PostgreSQL instead

**Next Step:** Run load tests with current fixes, measure write impact

---

## 📋 PLANNED FIXES

### 3. Add ReadHeaderTimeout (Slowloris Protection)
**Status:** 📋 Not started  
**Priority:** 🟢 LOW (security hygiene)  
**File:** `cmd/server/main.go`  
**Effort:** 5 minutes

**Current:**
```go
ReadTimeout:  15 * time.Second,
WriteTimeout: 15 * time.Second,
IdleTimeout:  60 * time.Second,
```

**Proposed:**
```go
ReadHeaderTimeout: 2 * time.Second,  // NEW - protects against Slowloris
ReadTimeout:       15 * time.Second,
WriteTimeout:      15 * time.Second,
IdleTimeout:       60 * time.Second,
```

**Why:**
- Protects against slow HTTP header attacks
- Prevents resource exhaustion from malicious clients
- Standard security best practice

**Impact:** No performance change, security improvement only

---

### 4. SMD Client Retry Logic (Optional)
**Status:** 📋 Nice to have  
**Priority:** 🟡 MEDIUM (low urgency due to caching)  
**File:** `pkg/smdclient/http_client.go`  
**Effort:** 30 minutes

**Current:**
```go
client: &http.Client{
    Timeout: 10 * time.Second,  // Hardcoded, no retry
}
```

**Proposed:**
```go
// Add retry wrapper similar to cloud-init-server
func (c *HTTPClient) IDfromIPWithRetry(ip string, maxRetries int) (string, error) {
    for attempt := 0; attempt < maxRetries; attempt++ {
        id, err := c.IDfromIP(ip)
        if err == nil {
            return id, nil
        }
        if !isRetryable(err) {
            return "", err
        }
        time.Sleep(backoff(attempt))  // 100ms, 200ms, 400ms
    }
    return "", fmt.Errorf("max retries exceeded")
}
```

**Why:**
- SMD client has hardcoded 10s timeout, no retry
- Cloud-init-server already has retry logic (we can copy pattern)
- Cache hit rate should be >90%, so this is low priority

**Impact:** Slight improvement for cache misses during SMD overload

**Tradeoff:** Adds complexity, may hide SMD performance issues

**Decision:** **DEFER** - Monitor cache hit rate first. If >95%, not needed.

---

## 🔬 OPTIONAL DEEP OPTIMIZATIONS

### 5. Pre-warm SMD Cache (Advanced)
**Status:** 📋 Optional  
**Priority:** 🟢 LOW (optimization, not fix)  
**Effort:** 1-2 hours

**Idea:** Add `/admin/cache/prewarm` endpoint that bulk-loads all nodes from SMD before boot storm

**Benefit:** 100% cache hit rate during boot, eliminating SMD as bottleneck

**Implementation:**
```go
// pkg/smdclient/integration_service.go
func (s *SMDIntegrationService) Prewarm(ctx context.Context) error {
    components, err := s.backend.ListComponents()
    if err != nil {
        return err
    }
    // Populate cache
    for _, comp := range components {
        s.cacheComponent(comp)
    }
    return nil
}
```

**When to use:** If smoke tests show cache misses causing issues

---

### 6. Switch to Database Backend (Major Change)
**Status:** 📋 Future work  
**Priority:** 🟢 LOW (if file backend is fine)  
**Effort:** 4-8 hours

**When needed:** If file backend shows global lock contention

**Migration Path:**
1. Fabrica already supports Ent backend (PostgreSQL)
2. Update `cmd/server/main.go` to use `storage.InitEntBackend()`
3. Add `--db-url` flag
4. Re-run load tests

**Benefit:** Row-level locking, better concurrency under write load

**Tradeoff:** Operational complexity (requires PostgreSQL)

---

## Testing Strategy

### After Each Fix

1. **Unit tests:** `go test ./pkg/... -race`
2. **Smoke test:** `./load-tests/quick-verify.sh` (30s)
3. **Full suite:** `./load-tests/run-all.sh` (staged + storm)

### Success Criteria

| Test | Target | Current Blocker |
|------|--------|-----------------|
| Smoke (10 VUs) | P99 < 500ms, 0% failure | ✅ Should pass |
| Staged (1K→10K) | P99 < 1s, <1% failure | ⏳ WG fix should help |
| Storm (10K concurrent) | P99 < 2s, <1% failure | ⏳ WG fix should fix |

---

## Recommended Next Steps

1. ✅ **DONE:** Async persistence (WireGuard fix)
2. ⏳ **NOW:** Wait for file backend analysis results
3. ✅ **THEN:** Run full load tests to establish new baseline
4. 📋 **IF NEEDED:** Add ReadHeaderTimeout (5 min, always good)
5. 📋 **IF NEEDED:** Investigate file backend (depends on #2)
6. 📋 **OPTIONAL:** SMD retry logic (low priority due to cache)

---

## Decision Tree

```
Run load tests with WG fix
  ├─ Storm test passes (<1% failure)? 
  │  ├─ YES → ✅ DONE! Ship it.
  │  └─ NO → Check failure cause
  │     ├─ File I/O bottleneck? → Fix #2 (file backend)
  │     ├─ SMD timeouts? → Fix #4 (retry logic)
  │     └─ Unknown? → Profile with pprof
  └─ Smoke test fails?
     └─ Investigate immediately (regression)
```

---

**Status:** 1/4 fixes complete, waiting on exploration results  
**Next:** File backend concurrency analysis

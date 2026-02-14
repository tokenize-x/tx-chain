# PSE Community Distribution: Architecture Comparison

**Document Type:** Technical Architecture Decision  
**Date:** 2024  
**Status:** For Team Discussion  
**Audience:** Engineering team, system architects

---

## Executive Summary

The PSE module's batched community reward distribution faces a fundamental architectural choice for handling large-scale delegator payouts while enabling concurrent scoring periods.

**Current Implementation (Single-Buffer with Copy):**
- Snapshots all scores into temporary `CommunityScores` map when job starts
- Payouts iterate this copied data across multiple blocks
- Next period scoring starts immediately on fresh stores
- **Trade-off:** Minimal code change, but creates temporary state duplication

**Proposed Double-Buffer Architecture:**
- Maintains two scoring buffers (A and B), swapping between them
- Payouts read directly from frozen buffer (no copy)
- New scores accumulate in active buffer in parallel
- **Trade-off:** More complex routing logic, but eliminates state copy entirely

This document provides side-by-side technical comparison, implementation details, and decision framework.

---

## Problem Context

### The Challenge
1. **Large delegator sets** (100K+) need reward distribution
2. **Scalability requirement:** Process in batches (1000 delegators/block = ~100 blocks for 100K)
3. **Concurrency requirement:** Next scoring period must start immediately, can't wait for payouts to finish
4. **State efficiency:** Minimize temporary state bloat during payout window

### Current Constraints
- Community rewards calculated from: `DelegationTimeEntries` + `AccountScoreSnapshot`
- Batching requires storing progress across blocks
- Next period needs fresh stores, can't reuse current period's data

---

## Implementation 1: Single-Buffer with Copy (Current)

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    PSE Module State                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Period N (being paid out):                               │
│  ├─ CommunityScores [addr] → score                       │
│  │  └─ Copy of DelegationTimeEntries+AccountScoreSnapshot │
│  │     (frozen at Block N, used for payout)               │
│  │                                                         │
│  Period N+1 (accumulating):                              │
│  ├─ DelegationTimeEntries [addr] → entry (fresh)         │
│  ├─ AccountScoreSnapshot [addr] → score (fresh)          │
│  │  └─ Receiving updates from hooks (active)             │
│  │                                                         │
│  Job tracking:                                            │
│  └─ CommunityDistributionJob                             │
│     ├─ processed_entries                                 │
│     ├─ next_address (cursor)                             │
│     └─ references CommunityScores for payout             │
│                                                         │
└─────────────────────────────────────────────────────────────┘
```

### When & How the Job Starts

**Trigger:** Module's `EndBlock` checks if scheduled distribution time has arrived. When it does, and no job is already running, it initiates `StartCommunityDistributionJob`.

**What happens:**
1. All existing scores from current period are read and calculated
2. These scores are copied into `CommunityScores` map (frozen state for payout)
3. Fresh stores (`DelegationTimeEntries`, `AccountScoreSnapshot`) are reset to empty
4. A `CommunityDistributionJob` is created tracking progress (processed entries, cursor address)
5. New scoring immediately begins accumulating in the fresh stores

**Timeline:**
- **Block N (trigger):** Job starts, scores copied, stores reset
- **Blocks N+1 to N+K:** Each block processes ~1000 addresses from frozen `CommunityScores`
- **Block N+K:** Last batch processed, job removed, `CommunityScores` cleared
- **Blocks N+1 to N+K (parallel):** New period accumulates scores independently

**Concurrent scoring:** While payouts happen from frozen data, delegation/staking hooks update the fresh stores. No waiting needed.

### Memory Profile - 30K Delegators Example

| Block | CommunityScores | DelegationTimeEntries | AccountScoreSnapshot | Total Keys | Notes |
|-------|-----------------|----------------------|----------------------|-----------|-------|
| N-1   | 0               | 30K (current period) | 30K                  | 60K       | Before distribution |
| N     | 30K (copy)      | 30K (reset)          | 0 (cleared)          | 60K       | Snapshot copied |
| N+1   | 30K (read)      | 30K (accumulating)   | ~1K (new scores)     | 61K       | Payouts started |
| N+15  | 30K (read)      | 30K (accumulated)    | ~15K (new scores)    | 75K       | Peak pressure |
| N+29  | 0 (cleared)     | 30K (full period)    | 30K (full period)    | 60K       | Payout complete |

**State pressure:** Constant 60K keys, no spike, linear growth of new period.

### State Store Structure

```
Stores:
├── DelegationTimeEntries
│   └── key:   [prefix][addr]
│       value: DelegationTimeEntry{delegation, lastChangedUnixSec}
│
├── AccountScoreSnapshot
│   └── key:   [prefix][addr]
│       value: score
│
└── CommunityScores  ← NEW MAP FOR COPY
    └── key:   [prefix][addr]
        value: score (copied from snapshot calculation)
        
CommunityDistributionJob (singular)
    ├── processed_entries: uint64
    ├── next_address: string
    └── (total amounts from params)
```

### How It Works in Practice

**Snapshot creation:** When trigger fires, calculate all scores from current data and store a copy. This copy is frozen and used for all 100 blocks of payout processing.

**Batch processing:** Each block, pull 1000 addresses from the frozen copy, calculate their payout amounts, transfer coins. Return cursor position to resume next block.

**Guard:** Before processing scheduled distributions, check if a community job is active. If yes, defer (wait until job finishes).

**Cleanup:** When all addresses processed, delete the copy and remove the job.

### Advantages

1. **Minimal code changes:** Only adds new CommunityScores map and copy logic
2. **Simple mental model:** Old period's data in CommunityScores, new period's data in fresh stores
3. **Fault tolerance:** Snapshot is explicit, easy to verify/debug
4. **Backward compatible:** No changes to existing stores or hooks
5. **Works for moderate scales:** Suitable for 100K-500K delegators

### Disadvantages

1. **State bloat at snapshot time:** O(n) copy operation creates temporary pressure
2. **Not optimal for massive delegator sets:** At 1M delegators, copy could spike 180MB+ in single block
3. **Extra storage:** Maintains separate CommunityScores map (not just another buffer of existing stores)
4. **Write pressure at snapshot:** All O(n) copy writes happen in single block

---

## Implementation 2: Double-Buffer Architecture (Proposed)

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    PSE Module State                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Buffer A (Active - for new scoring):                      │
│  ├─ DelegationTimeEntriesA [addr] → entry                 │
│  ├─ AccountScoreSnapshotA [addr] → score                  │
│  └─ Receiving updates from hooks                          │
│                                                             │
│  Buffer B (Frozen - for payout):                          │
│  ├─ DelegationTimeEntriesB [addr] → entry                 │
│  ├─ AccountScoreSnapshotB [addr] → score                  │
│  └─ NOT modified during payout processing                 │
│                                                             │
│  Control:                                                  │
│  ├─ ActiveBuffer: pointer to A                            │
│  ├─ PayoutBuffer: pointer to B                            │
│  └─ CommunityDistributionJob:                             │
│     ├─ payout_buffer_id: "B"                              │
│     └─ processed_entries / next_address                   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### When & How the Job Starts

**Trigger:** Same as Single-Buffer—`EndBlock` detects scheduled time has arrived.

**What happens (different approach):**
1. No score copy. Instead, buffers are *swapped*
2. Old period's data stays in frozen Buffer B (used for payout)
3. Buffer A (fresh, empty) becomes active for new scoring
4. A `CommunityDistributionJob` is created pointing to Buffer B (frozen, not a copy)
5. New scoring immediately begins in Buffer A

**Timeline:**
- **Block N (trigger):** Buffers swapped (A active, B frozen), job points to B
- **Blocks N+1 to N+K:** Each block reads from frozen Buffer B, no copy needed
- **Block N+K:** Last batch processed from B, job removed, Buffer B cleared
- **Blocks N+1 to N+K (parallel):** Buffer A accumulates new scores independently

**Key difference:** No copying of 30K scores into separate state—payout reads directly from already-existing frozen Buffer B. Buffers swap atomically.

### Memory Profile - 30K Delegators Example

| Block | Buffer A Keys | Buffer B Keys | Total Keys | Notes |
|-------|---------------|---------------|-----------|-------|
| N-1   | 30K (active)  | 0             | 30K       | Before distribution |
| N     | 0 (reset)     | 30K (frozen)  | 30K       | Buffers swapped, no copy |
| N+1   | ~1K (new)     | 30K (read)    | 31K       | Payouts started |
| N+15  | ~15K (new)    | 30K (read)    | 45K       | Growing new period |
| N+29  | 30K (full)    | 0 (cleared)   | 30K       | Payout complete |

**State pressure:** No spike. Smooth 30K→45K→30K curve (vs. flat 60K).

### State Store Structure

```
Stores:
├── DelegationTimeEntriesA
│   └── Same structure as current DelegationTimeEntries
│
├── DelegationTimeEntriesB
│   └── Duplicate prefix, same structure
│
├── AccountScoreSnapshotA
│   └── Same structure as current AccountScoreSnapshot
│
├── AccountScoreSnapshotB
│   └── Duplicate prefix, same structure
│
├── ActiveBufferID (module state)
│   └── Stores current active buffer: 0 for A, 1 for B
│
└── CommunityDistributionJob
    ├── payout_buffer_id: 0 or 1
    ├── processed_entries: uint64
    └── next_address: string
```

### How It Works in Practice

**Buffer swap:** When trigger fires, atomically swap which buffer is active (A) and which is frozen for payout (B). No copying—just pointer changes.

**Batch processing:** Each block, read 1000 addresses from frozen Buffer B, calculate payouts, transfer coins. Buffer B never changes during this window.

**Parallel scoring:** Hooks write to active Buffer A while payouts read from frozen Buffer B. No contention, no locking needed.

**Cleanup:** When all addresses processed, clear frozen Buffer B and remove the job. Buffer A remains active for next cycle.

**Routing:** All hooks and queries check which buffer is currently active, then read/write accordingly. No copy, direct access.

### Advantages

1. **No state copy:** Data remains in original frozen buffer, no O(n) copy operation
2. **Better scalability:** 1M delegators doesn't spike state, just steady 1M→1.5M curve
3. **Write pressure distributed:** No single-block copy, just lock-free reads
4. **Elegant buffer pattern:** Classic double-buffer works well for this use case
5. **Predictable performance:** No surprise write load at snapshot time

### Disadvantages

1. **Code complexity:** Requires routing logic in all hooks and getters
2. **Store multiplication:** Must maintain 4 stores instead of 2 (2× state storage)
3. **Query complexity:** Queries must know which buffer is active
4. **Migration cost:** Requires state migration for existing networks
5. **Debugging complexity:** State spread across two prefixes

---

## Detailed Comparison Table

| Category | Single-Buffer + Copy | Double-Buffer |
|----------|---------------------|---------------|
| **Snapshot operation** | Copy all scores to CommunityScores map | No copy, use existing frozen buffer |
| **Payout reads from** | Separate CommunityScores map | Original DelegationTimeEntriesB/AccountScoreSnapshotB |
| **State keys during payout** | ~60K (CommunityScores + new period buffers) | ~45K (both buffers) |
| **Peak state spike** | None, flat 60K | None, smooth curve |
| **Write pressure at snapshot** | High (O(n) writes in one block) | Low (just pointer change) |
| **Store count** | 3 (DelegationTimeEntries, AccountScoreSnapshot, CommunityScores) | 5 (4 buffers + control) |
| **Hook routing** | Simple (single active store) | Complex (conditional buffer selection) |
| **Code lines** | ~200 | ~400 |
| **Fault tolerance** | Explicit copy in state (verifiable) | Pointer-based swap (implicit) |
| **Query implementation** | Query active store directly | Query through ActiveBuffer router |
| **Scalability at 1M delegators** | O(n) copy could spike ~180MB | Smooth organic growth |
| **Cleanup** | Remove CommunityScores map | Clear frozen buffer in-place |
| **Migration needed** | None (new collections only) | Yes (state reorganization) |
| **Can run existing code alongside** | Yes (new addition) | Maybe (requires query updates) |

---

## Performance Analysis

### Block Execution Time

**Single-Buffer (at snapshot block):**
```
Copy operation: ~200ms (30K addresses × 6μs/write)
Job creation: ~5ms
Total overhead: ~205ms
→ Acceptable for normal block time (5-6s)
```

**Double-Buffer (at swap block):**
```
Buffer swap: <1ms
Job creation: ~5ms
Total overhead: ~6ms
→ Negligible
```

**Batch processing (every block during payout):**
```
Both: Read 1000 entries (1-2ms) + 1000 transfers (50-100ms)
Total: 50-100ms per block
→ Same for both architectures
```

### State Growth Curve

**Single-Buffer - 100K delegators:**
```
Before: 100K keys
At snapshot (block N): 200K keys (100% increase)
After snapshot copy: 200K keys → 100K keys cleaned after payout
Curve: ▌ → ▓ → ▌  (spike then drop)
```

**Double-Buffer - 100K delegators:**
```
Before: 100K keys (in Buffer A)
At swap (block N): Still 100K keys (now in Buffer B, A reset to 0)
During payout: 0-100K keys (Buffer A growing, Buffer B frozen at 100K)
After payout: 100K keys (Buffer A full, Buffer B cleared)
Curve: ▌ → ▌ → ▓ → ▌  (smooth, no spike)
```

---

## Real-World Scaling Examples

### Scenario 1: 50K Delegators

```
Single-Buffer:
├─ Snapshot: 280 blocks @ 1000/batch
├─ State copy: ~50 concurrent map operations
├─ Peak keys: 100K
└─ Write load: ~200ms in one block

Double-Buffer:
├─ Payout: 50 blocks @ 1000/batch
├─ State copy: 0 operations
├─ Peak keys: 50K
└─ Write load: <1ms in swap block
```

**Verdict:** Both acceptable. Single-Buffer simpler.

### Scenario 2: 500K Delegators

```
Single-Buffer:
├─ Snapshot: 500 blocks @ 1000/batch
├─ State copy: ~500 concurrent map operations (!)
├─ Peak keys: 1M
└─ Write load: ~2s in one block (!)

Double-Buffer:
├─ Payout: 500 blocks @ 1000/batch
├─ State copy: 0 operations
├─ Peak keys: 500K
└─ Write load: <1ms in swap block
```

**Verdict:** Double-Buffer preferred. Single-Buffer has write pressure.

### Scenario 3: 1M+ Delegators (Future)

```
Single-Buffer:
├─ Snapshot: 1000+ blocks @ 1000/batch
├─ State copy: ~1000 concurrent operations (worse!)
├─ Peak keys: 2M
├─ Write load: ~4s in one block (!)
└─ Risk: Block producer timeout, missed blocks

Double-Buffer:
├─ Payout: 1000+ blocks @ 1000/batch
├─ State copy: 0 operations
├─ Peak keys: 1M (stable)
└─ Write load: <1ms in swap block
└─ Risk: None
```

**Verdict:** Double-Buffer required. Single-Buffer infeasible.

---

## Decision Framework

### Choose Single-Buffer if:

✅ **Current delegator count:** < 200K  
✅ **Team priority:** Ship fast, minimize complexity  
✅ **Load tolerance:** Can absorb ~200-500ms write spike  
✅ **State pressure:** Observable but acceptable  
✅ **Team experience:** Prefers explicit snapshots over pointer swaps

### Choose Double-Buffer if:

✅ **Current delegator count:** > 300K or projected > 500K  
✅ **Team priority:** Scalability, future-proof  
✅ **Load tolerance:** Must minimize block time variance  
✅ **State pressure:** Need predictable query latency  
✅ **Team experience:** Comfortable with buffer management patterns

### Choose Hybrid (Migrate Later) if:

✅ **Want practical hybrid:** Ship Single-Buffer now, upgrade path to Double-Buffer  
✅ **Staged rollout:** Test Single-Buffer under load, migrate if needed  
✅ **Incremental migration:** Old jobs use Single-Buffer, new use Double-Buffer  
✅ **Risk mitigation:** Keep existing code, add new logic alongside

---

## Migration & Upgrade Path

### Path A: Start with Single-Buffer, Migrate on Demand

```
Timeline:
├─ v1.0: Single-Buffer + Copy (current proposal)
│   ├─ Works for 50K-300K delegators
│   └─ Shipped to mainnet
│
├─ v1.5: Add Double-Buffer support (parallel)
│   ├─ New DelegationTimeEntriesA/B stores
│   ├─ Keep Single-Buffer logic for old jobs
│   └─ Canary: Run Double-Buffer on testnet
│
├─ v2.0: Migrate to Double-Buffer by default
│   ├─ New jobs use Double-Buffer
│   ├─ Old Single-Buffer jobs finish normally
│   └─ Deprecate CommunityScores map
│
└─ v3.0: Remove Single-Buffer code
    └─ Cleanup only if no old jobs active
```

**Advantage:** No rush, real data on scalability, safe migration.

### Path B: Implement Double-Buffer from Start

```
Timeline:
├─ v1.0: Double-Buffer (more initial work)
│   ├─ All new jobs use optimized buffers
│   ├─ No legacy Single-Buffer code
│   └─ Future-proof from day 1
│
└─ Ongoing: Incremental performance tuning
    └─ No major refactors needed
```

**Advantage:** Simpler long-term, no migration debt.

---

## Recommendation for Team Decision

### **Recommended Approach: Path A (Start with Single-Buffer)**

**Rationale:**
1. **Time-to-market:** Issue is time-sensitive for upcoming scoring season
2. **Validation loop:** Single-Buffer will work and provide real load data
3. **Risk mitigation:** Single-Buffer is simpler, easier to debug issues
4. **Migration exists:** Double-Buffer path documented and ready if needed
5. **Team bandwidth:** Can ship now, upgrade later with full context

**Decision points for escalation:**
- If delegator count >> 300K at launch → consider Double-Buffer now
- If state query latency issues appear → trigger Double-Buffer migration
- If block times spike > 10% → review snapshot performance

**Ship date:** Target with Single-Buffer, Double-Buffer as follow-up enhancement.

---

## Conclusion

Both architectures solve the core problem of batched community distributions with concurrent scoring. The choice reflects different trade-offs:

- **Single-Buffer (Current):** Pragmatic choice for near-term needs, works well for moderate scales
- **Double-Buffer (Proposed):** Architectural excellence, optimized for massive scales

**The contingency plan (migrate later) is realistic and recommended:** Capture real-world load data with Single-Buffer, upgrade to Double-Buffer when scaling demands justify the engineering effort.

---

## Appendix: Benchmarking Checklist

For future migration decision, track these metrics:

- [ ] Peak state keys during payout window
- [ ] Block execution time (especially snapshot block)
- [ ] Average query latency during payout
- [ ] Delegator count growth rate
- [ ] Node validator's available memory
- [ ] Block time variance (histogram)

**Re-evaluate Double-Buffer if:** Any metric exceeds expected bounds after Single-Buffer deployment.

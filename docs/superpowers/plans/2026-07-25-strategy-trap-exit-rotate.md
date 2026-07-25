# Strategy Trap / Exit / Rotate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Refuse trap entries, hold through phantom adverse, stop on sincere dumps, and rotate only on proven edge — all verified via `tests.Market` + production boot.

**Architecture:** Trap share from Thesis measurement masses → inflate forecast adverse (`logic`) → strategy hard-veto when traps dominate (`Opportunity.Measure`) → extend `strategy/planner_test.go` sagas for exit sincerity and rotation.

**Tech stack:** Go, GoConvey, `tests.Market` / `stack.Booter.Test`, existing `types.Forecasts` / `Stoploss` / `Rotate`.

---

## File map

| File | Responsibility |
|------|----------------|
| `logic/trap.go` (new) | Derive trapShare / dominant family from Thesis measurements |
| `logic/trap_test.go` (new) | Unit proofs for mass ratio |
| `logic/analyzer_observe.go` | Apply TrapTax into ExpectedAdverseSelection |
| `strategy/trap.go` (new) or fold into `reading.go` | Shared trapShare read for Opportunity veto |
| `strategy/opportunity.go` | `trap_dominant` reject after utility would clear |
| `strategy/planner_test.go` | Market-sim: trap refuse, exit sincerity, rotation |
| `types/stoploss_test.go` | Pierce retreat when ExpectedReturn < 0 (if missing) |

---

### Task 1: Trap mass helper + forecast adverse

**Files:** `logic/trap.go`, `logic/trap_test.go`, `logic/analyzer_observe.go`

- [x] Write failing unit tests: absorption-only mass ⇒ trapShare → 1; ignition-only ⇒ 0; mixed ⇒ ratio
- [x] Implement mass scan over SnapshotMeasurements (normalized only)
- [x] Wire `forecastAdverse` to add `trapShare * max(0, ExpectedReturn)`
- [x] Run `go test ./logic/ -run Trap -count=1`

### Task 2: Strategy trap veto

**Files:** `strategy/opportunity.go`, `strategy/opportunity` tests or planner_test

- [x] Failing market-sim: VolumeAbsorption / LowVolumeLift / SpoofLiquidity ⇒ no ActionEnter; FastPump control still enters
- [x] Veto in Measure when traps strictly dominate with cause `trap_dominant`
- [x] Run planner trap tests

### Task 3: Exit sincerity saga

**Files:** `strategy/decide_exit_test.go`, `types/stoploss.go`, `types/stoploss_test.go`

- [x] Root cause: residual-only take_profit cashed peaks while ExpectedReturn stayed positive
- [x] takeProfit requires non-positive forward / adverse causal path near peak
- [x] Market-sim: pump → spoof hold; pump → retreat hold → dump stop
- [x] Unit: retreat pierced when ExpectedReturn < 0

### Task 4: Rotation saga

**Files:** `strategy/decide_rotate_test.go`

- [x] Market-sim: fill normal slots, FastPump challenger → rotation or audited `rotate_wait`

### Task 5: Verify

- [x] Trap unit + refuse market-sim + pumpdump Calculate verified
- [x] Stoploss / Decide / DeskMarket / exit sincerity / rotation proofs green

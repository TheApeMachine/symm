# Rigorous Domain Corrections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make domain validity part of every primitive named in `rigorous_correctness_analysis.md`: reject invalid samples/denominators/logs/radicands with typed errors; inject PRNG state; use one quantile estimator.

**Architecture:** Shared correction policy across symm/nomagique/datura. Go uses `errnie.Err(errnie.Validation, …)` (or `UnprocessableContent` where that package already does). TypeScript throws a small `DomainError` class. No epsilon/zero/NaN fallbacks for undefined statistics. Scale-aware sqrt tolerance only for FP cancellation.

**Tech Stack:** Go + errnie, TypeScript/React frontend, gonum where already used.

## Global Constraints

- No magic-number clamps as substitutes for undefined ratios (`+1e-12`, `Math.max(n, 1)` as statistical defense).
- Do not invent numeric fallbacks for empty samples or zero denominators.
- Prefer methods on types; Godoc `/**/` comments on types/methods; `//` inline.
- Tests + benchmarks required for signal/statistic changes; paste stdout.
- Never discard git working tree state.

## File map

| Area | Files |
|---|---|
| TS domain helper | `symm/frontend/src/lib/domain.ts` (new) + tests |
| TS UI sites | `charts.tsx`, `xray-view.ts`, `xray-draw.ts`, `dashboard-rail.tsx`, `health.tsx`, `kernel/inspector.tsx`, `kernel/row.tsx` |
| symm Go | `logic/causal.go`, `logic/resonance.go`, `logic/manifold/inject.go`, `strategy/planner.go`, `kraken/websocket/simulator.go` |
| nomagique | cohort_sample, causal/table, causalstory, eigenmode_toroidal, pga, hawkes/*, learning/resonance, learning/rls, algorithm/streams, correlation/hayashi, mcts/*, statistic/quantile, statistic/ridge |
| datura | `dmt/cognitive_engine_runtime.go` |

### Task 1: TypeScript DomainError + frontend sites

**Files:** Create `frontend/src/lib/domain.ts`; modify listed TS sites; mirror tests.

- [ ] Add `DomainError` with codes: `empty_sample`, `insufficient_sample`, `zero_denominator`, `non_positive_base`, `log_domain`, `sqrt_domain`
- [ ] Empty-sample: `charts.tsx` position row height; `xray-view.ts` mean absolute error
- [ ] Sample n>=2: `inspector.tsx`, `row.tsx` index normalization
- [ ] Dynamic denominators: `charts.tsx` index/denom; `dashboard-rail.tsx` capital; `health.tsx` bar/total; `xray-draw.ts` span
- [ ] Tests covering throw paths; run vitest for touched files

### Task 2: symm Go domain + PRNG

**Files:** causal, resonance, planner, manifold/inject, simulator (+ tests)

- [ ] `causal.observe` / `resonance.learnReturn`: require mid/reference > 0 and log arg > 0 before `math.Log`
- [ ] `planner.Decide`: if `freeTotal` used as denominator, require > 0 and error (or skip with typed decision cause — no silent divide); remove dead paths that imply undefined capital ratios
- [ ] `carrierSpeed`: scale-aware radicand check
- [ ] `Simulator`: inject `*rand.Rand` from recorded seed; store seed on simulator/result
- [ ] Tests + benches for changed statistic paths

### Task 3: nomagique domain ledger

**Files:** all nomagique rows in analysis

- [ ] Denominators: cohort_sample, causal/table, causalstory, eigenmode normalizeVec, pga MotorFromAxisAngle/Interpolate, hawkes bounds/fit_context, resonance Learn/stateGradients/updatePrecision (remove `+1e-12` / PrecisionEps-as-fallback; require positive variance/norm), rls observeOnce
- [ ] Logs: streams/hayashi varianceSum, softplus/inverseSoftplus domains, hawkes gradient lambda, mcts Visits
- [ ] Sqrt: machine-epsilon helpers + any subtractive radicands with scale-aware tol
- [ ] Quantile: `statistic/quantile.go` and `causal/table.go` percentile → `(n-1)*p` linear interp after sort + p validation
- [ ] MCTS simulate: injected `*rand.Rand` from run seed recorded on result
- [ ] Tests + benches

### Task 4: datura cognitive runtime

**Files:** `dmt/cognitive_engine_runtime.go` + tests

- [ ] `buildContextTrainingMutations`: reject empty parent sample instead of substituting `nextCount`
- [ ] `writeArtifact`: require `parentWeight.Count+1 > 0` (non-negative count contract) before reciprocal; typed error if violated
- [ ] Tests

### Task 5: Verify

- [ ] Run package tests/benchmarks for every touched package; paste literal stdout

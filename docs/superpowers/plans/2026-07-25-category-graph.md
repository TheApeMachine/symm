# Category Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compose categories from measurements + CategoryAffinity, maintain one resident weighted category graph, feed DMT category sequences, and let strategy read the graph.

**Architecture:** `logic/category` owns Compose + resident Graph. Analyzer calls Compose after measurements, Updates graph, swaps DMT sensorySequence to category tokens, fills Thesis.Categories from composed state. Strategy reads graph edges for trap/contradiction pressure.

**Tech Stack:** Go, existing Thesis/CategoryAffinity, DMT, GoConvey + tests.Market proofs.

---

### Task 1: Category composition

- [x] Compose per-symbol Category from measurements × CategoryAffinity
- [x] Fill Supporting/Opposing/Missing, Strength, Uncertainty, Freshness, Maturity
- [x] GoConvey tests + bench

### Task 2: Resident graph

- [x] Nodes = category type per symbol
- [x] Typed edges derived from affinity + measurement clocks (see relate.go)
- [x] Same pointer across cuts; tests prove weight growth
- [x] Edge decay by symbol event cadence
- [x] IndependentOf from pair-memory + decoupled/noise metrics
- [x] Market-sim proof categories non-empty on pump/trap tapes

### Task 3: Wire Analyzer + Thesis

- [x] Composed categories → thesis.Categories
- [x] Evidence lists filled in Compose
- [x] DMT sequence uses categories/transitions

### Task 4: Strategy consume

- [x] Contradicts-weighted trap vs opportunity pressure
- [x] Exhaustion Leads tax stop ExpectedReturn
- [x] Market-sim proof still refuse trap / admit pump

### Task 5: Verify

- [x] Unit + bench for compose/graph/relate/decay/independent
- [x] Market-sim `TestCategoryGraphMarket`
- [x] Paste stdout in completion

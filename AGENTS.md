# AGENTS.md

You are a lazy senior developer. Lazy means efficient, not careless. The best code is the code never written.

Before writing any code, stop at the first rung that holds:

1. Does this need to be built at all? (YAGNI)
2. Does the standard library already do this? Use it.
3. Does a native platform feature cover it? Use it.
4. Does an already-installed dependency solve it? Use it.
5. Can this be one line? Make it one line.
6. Only then: write the minimum code that works.

Rules:

- No abstractions that weren't explicitly requested.
- No new dependency if it can be avoided.
- No boilerplate nobody asked for.
- Deletion over addition. Boring over clever. Fewest files possible.
- Question complex requests: "Do you actually need X, or does Y cover it?"
- Pick the edge-case-correct option when two stdlib approaches are the same size, lazy means less code, not the flimsier algorithm.
- Mark intentional simplifications with a `ponytail:` comment. If the shortcut has a known ceiling (global lock, O(n²) scan, naive heuristic), the comment names the ceiling and the upgrade path.

Not lazy about: input validation at trust boundaries, error handling that prevents data loss, security, accessibility, the calibration real hardware needs (the platform is never the spec ideal, a clock drifts, a sensor reads off), anything explicitly requested. Lazy code without its check is unfinished: non-trivial logic leaves ONE runnable check behind, the smallest thing that fails if the logic breaks (an assert-based demo/self-check or one small test file; no frameworks, no fixtures). Trivial one-liners need no test.

---

## Project objective

**Maximize the wallet. Minimize the time to do so.**

Miracles are not expected. A best-effort, highly principled system is. The goal is to detect as many real opportunity types as the market presents — pumps, coils, exhaustion, liquidity vacuums, sector lifts, thin-book traps — and to act on them with dynamically derived thresholds only.

Failure after an honest principled try is acceptable. Failure from magic numbers, incomplete data sources, or comment blocks that do not match implementation is not.

---

## Measurements are not decisions

**Only `trader/crypto.go` (`Crypto`) makes decisions** — choosing which candidate action (if any) to dispatch and how to rank opportunity across symbols. Everything flows through that module for a reason: it is the single place that sees holdings, broker constraints, and the full candidate set.

### The funnel (end-to-end, no shortcuts)

Value must be traceable at every stage. No layer may collapse or guess what a later layer needs.

```
raw market data (websocket → dmt.Tree)
        ↓
signals.Measure (per origin: pumpdump, hawkes, …)
        ↓  measurement artifacts: category masses, confidence, strength, scalars, timestamp, replay fields
market.Story.Update → logic.Tree.Evaluate (playbook walks)
        ↓  candidate actions (logic.Action): proposed entries, exits, fractions — not yet committed
Crypto.Run (trader) — rank / choose highest-value candidate(s)
        ↓
broker.Desk — fills, stops, ratchets
```

| Stage  | Package / type                      | Output                                        | Decides?                  |
|--------|-------------------------------------|-----------------------------------------------|---------------------------|
| Ingest | `kraken/…` → tree                   | Raw role/scoped artifacts                     | No                        |
| Signal | `signal/*` → `Measure`              | `measurement` artifacts per symbol per origin | No — observational regime |
| Story  | `market/story.go` → `logic/tree.go` | **Candidate** `logic.Action` slice            | No — playbook proposes    |
| Trader | `trader/crypto.go`                  | **Decision** — which candidate(s) to execute  | **Yes — only here**       |

`Crypto.Run` today (`trader/crypto.go`): `measurements := crypto.signals.Measure(...)` → `actions := crypto.story.Update(measurements)` → **`// TODO(trader): choose among the candidate actions here`**. The desk currently dispatches what the story proposed without ranking. Completing that TODO is part of the project objective — not signal work.

### What measurements must carry (value tracking)

Playbook conditions and the trader need enough structure to evaluate **concurrent** patterns (same tick) and **sequential** patterns (across ticks) without magic shortcuts.

Each measurement artifact should expose (via `output.*` and replay fields on the artifact):

| Field                                 | Role                                                                                                  |
|---------------------------------------|-------------------------------------------------------------------------------------------------------|
| `category` / `category.N` / wire keys | Which story won on this origin                                                                        |
| `confidence`, `strength`              | How concentrated the distribution is (see `signal/dist`)                                              |
| `surprise`                            | When implemented: how unusual vs this symbol's recent measurement history                             |
| `elapsed`                             | When implemented: time since a prior category on **this origin** or since a cross-origin anchor event |
| `timestamp`                           | Wall-clock anchor for all interval math                                                               |
| Origin-specific scalars               | e.g. pumpdump `rvol`, hawkes λ/ρ — structured evidence, not just category index                       |

Whether the trader consumes raw scalars, `confidence`, or `surprise` is an implementation detail. What matters is that **the information exists on the artifact and in the tree** so nothing is invented downstream.

**No fixed intervals in signals or playbook.** Example: "frenzy should follow vertical ignition within an ideal window" — that window is **not** five minutes hardcoded. It is derived from observed cadence: median inter-measurement gap × interval budget (`statutil.MedianCadence`, `statutil.WindowDepth`) for that symbol, possibly scaled by cross-section peer cadence. The playbook compares `elapsed` operands to those derived bounds, or the trader ranks candidates by freshness of sequential stages.

### Sequential setups (example: multi-leg pump)

For a session with several rise/fall legs in a few hours (UNFI-style), an **ideal entry narrative** might be:

1. **pumpdump** Coiled Compression (setup)
2. **pumpdump** Vertical Ignition (structure breaks)
3. **hawkes** Consensus Frenzy (tape self-excitation confirms — not lonely volume)
4. Guards concurrent: toxicity not Bluff, fluid not Turbulent, liquidity not Extreme Scarcity

That is **sequential + concurrent** composition:

- **Sequential:** (1) then (2) then (3) on the timeline — stages may be ticks apart.
- **Concurrent:** guards must hold on the same tick as the final stage (entry).

`logic/rules/tree.yml` documents this intent (nested branches = "stage A fired, THEN stage B"; comments reference `sliceTimelineAfter`). **Implementation gaps (Jun 2025):**

- `logic/operand.go` does not resolve `SubjectElapsed` or `SubjectSurprise` yet — defined in `subject.go`, unsupported in comparisons.
- Playbook nesting currently re-evaluates **the same measurement batch** on child branches (`logic/branch.go`); cross-tick sequencing requires timeline state (prior matched stage + timestamp on the tree or in story) and measurement history per symbol — not a hardcoded delay.
- Hawkes frenzy is not yet in the ignition entry branch (comment in `tree.yml`: macro temperature scales hawkes gates separately).

Signals job: emit honest categories and timestamps every tick. Story job: express sequential/concurrent rules using those artifacts. Trader job: when multiple candidates fire, **rank by end-to-end expected value** (confidence, stage completeness, freshness, holdings, fees — all derived, not magic).

### Hawkes and the multi-leg swing (UNFI / SRM / SLX)

The screenshot sessions (spike → retrace → second leg → dump → grind) are not one pumpdump event. They are **trade-flow thermal cycles** — exactly the domain Hawkes names in its comment block:

| Chart phase                                      | Hawkes category (intended)                          | What it measures                                                         |
|--------------------------------------------------|-----------------------------------------------------|--------------------------------------------------------------------------|
| Vertical leg, one-sided aggression               | **Consensus Frenzy**                                | Buy (or sell) intensity feeding back — chain reaction, not lonely volume |
| Chop / consolidation between legs                | **Exogenous Drift** or low-intensity contested flow | Trades arrive without strong self-excitation — coiled, not ignited       |
| Top of leg 2, violent two-sided tape before dump | **Contested Saturation**                            | Spectral radius high, both sides excited — "boiling," unstable           |
| After dump, flow dies                            | **Flow Exhaustion**                                 | Intensities below background μ — thermal death, move out of steam        |
| Before leg 3 grind                               | Drift → rising Frenzy again                         | Sequential recovery of excitation                                        |

Hawkes is **not** a price forecaster and should not be documented or implemented as "predict the next pump." It measures whether **trade arrivals are self-exciting, saturated, organic, or dead** — the temperature and feedback loop of the tape. That is what swings up and down between legs: excitation builds, saturates, exhausts, rebuilds.

**Principled use:** pair Hawkes measurements **sequentially** with pumpdump (structure: coil / ignite / exhaust) and **concurrently** with exhaust (exit urgency) and toxicity (book honesty). A decision might require, for example, pumpdump Ignition **and** hawkes Frenzy **and not** hawkes Saturation — overheated saturation is a different story than a clean directional frenzy.

Current gap: hawkes implementation does not yet match its comment block (book confirmation unused, `sync.Map` history). The **semantic fit** with multi-leg swings is intentional; the wiring still belongs in Track B Phase 3.

### Signal roles in the stack (mental model)

| Layer                    | Packages                                           | Job                                                    |
|--------------------------|----------------------------------------------------|--------------------------------------------------------|
| **Structure / ignition** | pumpdump, fluid, depthflow                         | Where is price-volume-book relative to legs and coils? |
| **Tape thermodynamics**  | hawkes, cvd                                        | Is flow self-exciting, absorbing, or exhausted?        |
| **Honesty / exit**       | toxicity, exhaust                                  | Is the book real? Is the move hollow?                  |
| **Universe context**     | sentiment, correlation, liquidity, causal, leadlag | Sector lift, beta, rank, anchor lag                    |
| **Candidate actions**    | `market.Story` + `logic` playbook                  | Proposed entries/exits when walk conditions match      |
| **Decision**             | `trader/crypto.go` (`Crypto`)                      | Rank candidates; dispatch to `broker.Desk`             |

Websocket ingest (L2/L3/trade/ticker → tree) is infrastructure. **Measurement semantics come first; transport wiring later.**

---

These notes come from live-market observation (Kraken Pro, Jun 2025: UNFI, SRM, SLX, TITCOIN). They define how every signal in `signal/*/` must be reviewed and implemented. See also `nomagique/README.md` § Domain notes.

### The comment block is the spec

Each signal package opens with a comment block describing:

1. What it measures in isolation (derived metrics).
2. The semantic story each category tells.
3. A summary table mapping categories to indicators and market "feel".

That block is not documentation fluff. It is the acceptance criteria. Before merging signal work, an agent must verify:

- Every category in the table can actually win given the declared inputs.
- Every indicator in the table is computed from declared data sources.
- Categories describe **phases** or **regimes**, not one-shot boolean alarms.

### Market patterns we are chasing

**Pumps are multi-leg cycles, not single events.**

Observed on UNFI/EUR (1m): leg 1 ignition (~18:00), consolidation/coiling (19:00–21:00), leg 2 ignition (~21:00, larger), exhaustion/dump (~22:30), leg 3 grind (~04:00). SRM/USD and SLX/USD (15m) show the same shape: spike → retrace → second leg, with order-book sell walls at resistance.

The four pumpdump categories map to a **state machine**:

| Phase on chart                                              | Category that should dominate |
|-------------------------------------------------------------|-------------------------------|
| Flat / pre-move                                             | Low confidence                |
| First vertical + volume                                     | Vertical Ignition             |
| Consolidation, flat price, building volume, tightening book | Coiled Compression            |
| Steady grind, peer-aligned                                  | Organic Trend                 |
| Lift fading, post-peak                                      | Faded Exhaustion              |

A signal that fires once on the first +400% tick and ignores later legs fails the project objective.

**Two pump species must stay separate.**

| Species                            | Tell                                                                         | Required data                                                                        |
|------------------------------------|------------------------------------------------------------------------------|--------------------------------------------------------------------------------------|
| Vertical ignition (UNFI, SRM, SLX) | Price + executed volume vertical together; book has structure (walls, depth) | ticker + **trade** + **book** + cross-section                                        |
| Thin-book % pump (TITCOIN)         | Huge % on tiny USD volume; spread 10%+; hollow book; often delisting         | Same sources — peer dollar-volume rank and book spread **disqualify** false ignition |

Ticker-only scoring cannot implement the comment block faithfully.

### Required data sources (non-negotiable)

| Source                        | Role                                                             | Access pattern                                                                   |
|-------------------------------|------------------------------------------------------------------|----------------------------------------------------------------------------------|
| **Tree — prior measurements** | Per-symbol baselines, leg context, replay                        | Seek `measurement/{symbol}/{origin}/`; sort by timestamp; `statutil.WindowDepth` |
| **Tree — book ingest**        | Touch spread, depth, walls, coiling                              | Seek `book/{symbol}/…` at score time                                             |
| **Tree — level3 ingest**      | Per-order add/delete/fill, order age, bluff detection            | Seek `level3/{symbol}/…` at score time; **authenticated** ws-l3 or private REST  |
| **Tree — trade ingest**       | Interval executed volume, aggression                             | Seek `trade/{symbol}/…` within cadence window                                    |
| **CrossSection**              | Peer lift, peer precursor, breadth, idiosyncratic vs sector move | `Observe(row)` on every ticker row; peer stats for normalization                 |

**No local per-pair store.** The tree is the only history store. Measurements must persist replay fields (`volume`, `last`, `bookSpread`, `touchDepth`, `tradeVolume`, `legAnchorLow`, `legAnchorHigh`, `lastExhaustionStamp`, `timestamp`, …) so the next frame rebuilds state from the tree alone.

**Windows derive from timestamps.** Use `statutil.WindowDepth(stamps)` and `statutil.MedianCadence(stamps)`. Never hardcode seconds, tick counts, or "warmup" sample gates. The first observation uses itself as baseline (mean of one = value).

### L2 book vs L3 order book

Kraken **L2 `book`** frames aggregate quantity at each price level. They are enough for touch spread, top-of-book depth, and coarse imbalance. They are **not** enough for stories that require knowing *what happened to individual orders*.

**L3 `level3`** frames carry per-order events (`add`, `delete`, `modify`) with `order_id`, `limit_price`, `order_qty`, and timestamps (see `tests/fixtures/order.json` and the L3 example in § nomagique below). From L3 you can derive:

- **Cancel vs fill** — an order that disappears without a trade at that price is a cancel (toxicity's core metric).
- **Order age** — time since `add` at near-touch; "young large block that vanishes" is the bluff signature in the toxicity comment block.
- **Spoof shape** — deep-book orders that add and delete without trade confirmation (depthflow Spoof Trap).
- **Per-level thinning** — depth removed by deletes vs fills (exhaust Book Thinning).
- **Wall persistence** — SRM/SLX-style sell walls: aggregate by price, track add/delete churn (pumpdump resistance context).

L3 is **authenticated** ([Kraken historical data](https://docs.kraken.com/exchange/guides/general/historical-data): `POST /0/private/Level3`, `wss://ws-l3.kraken.com/v2`). There is **no public historical L3 backfill** — forward capture into the tree only, same as L2 `Depth`.

**Ingest gap (Jun 2025):** `kraken/public/endpoint.go` defines `WebSocketL3URL` and `Level3Channel`, but `kraken/public/websocket.go` subscribes to `book`, not `level3`. Wiring L3 websocket → tree (`role=level3`, scope=symbol) is prerequisite for principled toxicity and unlocks depthflow/exhaust/pumpdump book stories.

**Signals that need L3 for a faithful comment-block implementation:**

| Signal        | Why L3                                                                                                                                                                   |
|---------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **toxicity**  | **Primary.** Cancel-to-fill asymmetry and toxic near-touch levels require order-level add/delete vs trade tape — L2 qty delta alone cannot distinguish cancel from fill. |
| **depthflow** | Spoof Trap and multi-level shape; comment already admits L2 top-of-book is incomplete.                                                                                   |
| **exhaust**   | Book thinning and imbalance flip per level; deletes vs fills matter for "hollow" moves.                                                                                  |
| **fluid**     | Replenishment resistance and viscosity after consumption — order arrival/cancel rates at touch.                                                                          |
| **pumpdump**  | Sell/buy wall churn and coiled depth stacking; L2 walls are snapshots, L3 shows whether walls are spoofed.                                                               |
| **hawkes**    | Book imbalance confirmation in comment block — order-event intensity pairs naturally with trade excitation.                                                              |

Signals that remain **L2-or-ticker sufficient** for now: liquidity, sentiment, correlation, leadlag, causal (macro/shock may still want L2 spread), cvd (trade-only).

At score time: seek latest L3 state or replay recent `level3` events in the cadence window from the tree — same pattern as pumpdump `bookEnrichment` / `tradeEnrichment`, with order-level aggregation done inline in Go.

---

Session-long baselines **contaminate** after leg 1: leg 2 ignition looks "moderate" vs history that includes the first pump. Principled fix:

- Anchor precursor to **current leg** consolidation range (`legAnchorLow` / `legAnchorHigh` on measurement artifacts), not only tick-to-tick delta.
- When **Faded Exhaustion** dominates, record `lastExhaustionStamp` and reset local leg anchors on the next measurement.
- **Coiled Compression** is the highest-leverage pre-entry: moderate lift + low peer-relative precursor + book spread tightening — the 19:00–21:00 UNFI chop, not the vertical candle.

### Anti-patterns (learned the hard way)

- Fixed time windows (e.g. 60 seconds) copied from external repos.
- Scoring ticker summary fields and calling it "microstructure."
- `sync.Map` / in-memory pair history when the tree already shares state.
- Positive-only returns for dump detection — exhaustion needs lift decline **and** price rejection context.
- One-shot test fixtures (single spike) without multi-leg replay.
- Category masses merged invisibly (e.g. `trendMass + flatMass` with one wire key).
- Bare multipliers (`*2`, `(1-x)`) in classifiers without a statistic in the denominator.

### Implementation strategy (read this first)

**Measurements first, websocket later, decisions in logic.** See § Measurements are not decisions.

Three tracks run in parallel for **core signals**; a fourth track (**manifold**, **resonance**) is deferred. Do not skip ahead on signal logic before validation data exists.

**Track A — Validation data (Kraken public REST)**

Historical replay for price/volume/trade — not book. See [Kraken historical data](https://docs.kraken.com/exchange/guides/general/historical-data).

| Endpoint                               | Use                                           | Limit                                                     |
|----------------------------------------|-----------------------------------------------|-----------------------------------------------------------|
| `GET /0/public/OHLC`                   | Multi-leg shape, ignition/retrace/re-ignition | 720 candles/call; paginate with `last`                    |
| `GET /0/public/Trades`                 | Executed lift (RVOL ground truth)             | 1000 trades/call; `since` in nanoseconds                  |
| `GET /0/public/Depth`                  | L2 book snapshot                              | **Live only** — aggregated levels, not per-order          |
| L3 (`level3` websocket / private REST) | Per-order add/delete; cancel vs fill          | **Live only**, **authenticated** — no historical backfill |

Target pairs for fixtures: UNFI, SRM, SLX (vertical ignition + multi-leg), TITCOIN (thin-book trap). Insert into `dmt.Tree` with the same role prefixes websockets use; replay through `Measure`.

Book-dependent categories (Coiled Compression, toxic bluff, book thinning) require **forward websocket capture** (L2 minimum; **L3 for toxicity, depthflow spoof, order-age bluff**) or OHLC/trade-only approximations in backtests until live book history exists.

**Track A2 — L3 ingest (blocks principled toxicity)**

1. Subscribe authenticated ws-l3 (or private Level3) for traded symbols.
2. Write frames to tree with `role=level3`, same prefix convention as `book` / `trade`.
3. Extend toxicity Phase 1/3 to seek `level3/{symbol}/…` and score cancel/fill/age from order events + trade correlation.
4. Forward-capture during live sessions builds the only replay corpus (no REST backfill).

**Track B — Core signal hardening (12 packages, phased)**

These are the **`market.Signal`** implementations wired through `trader/signal.go` that score inline from tree ingest and emit category measurements for playbook walks. **`manifold` and `resonance` are excluded** — see Track D.

| Phase | Goal                                            | Packages                                               | Exit criterion                                                              |
|-------|-------------------------------------------------|--------------------------------------------------------|-----------------------------------------------------------------------------|
| **1** | Tree-only history; delete `sync.Map` as primary | toxicity → cvd → hawkes → depthflow → exhaust → causal | Cold-start test: rebuild baselines from tree measurements only              |
| **2** | Complete reference signal                       | pumpdump                                               | Leg anchors, exhaustion reset, thin-book gate, multi-leg Kraken replay test |
| **3** | Comment block = implementation                  | fluid, hawkes, toxicity, depthflow                     | Every promised data source wired or comment corrected                       |
| **4** | crossSection hygiene                            | fluid, hawkes, cvd, exhaust, leadlag                   | Require peer context where story needs it; remove dead param elsewhere      |
| **5** | File size + tests                               | fluid, causal, leadlag                                 | Under 400 lines/file; cadence-derived windows, not fixed warmup ticks       |

**Already aligned (maintain + fixtures):** liquidity, sentiment, correlation.

**Do not start Phase 3 on a package until Phase 1 is done for that package.** pumpdump Phase 2 is the template others copy.

**Track D — Field signals (deferred, separate function)**

**`manifold`** and **`resonance`** perform a different job than the core signals above. They are field/latent-space layers (3D field solver, sensory batch autoencoder), not direct microstructure category scorers in the same sense as pumpdump or toxicity.

- Do **not** block Track B on manifold or resonance.
- Do **not** apply the core-signal Phase 1–5 checklist to them until explicitly scheduled.
- **`resonance`** is not on `market.Signal` / not in `trader/signal.go`; **`manifold`** is wired in trader but treated as field infrastructure for now.
- Architecture, trader integration, and principled audit for these two will be decided in a **separate discussion** later.

**Track C — Playbook + trader (candidates → decisions)**

- **Story / playbook** (`market/story.go`, `logic/rules/tree.yml`): measurements in → **candidate actions** out. Still not decisions.
- **Crypto trader** (`trader/crypto.go`): candidates in → **choice** out → `broker.Desk`. This is the only decision point.

Playbook walks may require concurrent origins (same tick) and sequential stages (cross-tick). Signals must publish timestamps and (when wired) `elapsed` / `surprise` so intervals are **cadence-derived**, never hardcoded seconds.

Trader TODO: rank candidates by end-to-end value when multiple actions return from `story.Update`. Do not implement ranking inside individual signals.

---

### Signal audit — core signals (Jun 2025)

Legend: ✅ aligned · ⚠️ partial · ❌ gap

| Signal          | Story (categories)                            | Ingest        | crossSection  | State               | Book/trade @ score  | Verdict                                                         |
|-----------------|-----------------------------------------------|---------------|---------------|---------------------|---------------------|-----------------------------------------------------------------|
| **pumpdump**    | 4 ignition phases (coil→ignite→trend→exhaust) | ticker        | ✅ required    | ✅ tree              | ✅ seek              | ⚠️ missing leg anchors, thin-book gate, multi-leg tests         |
| **fluid**       | Laminar / turbulent / inertial / viscous      | ticker        | ❌ unused      | ✅ tree              | ❌ none              | ❌ comment promises book+trade; ticker proxies only; 405 lines   |
| **toxicity**    | Vacuum / bluff / hard support                 | book          | ❌ unused      | ⚠️ sync.Map         | ❌ L2 qty delta only | ❌ needs **L3** for cancel/fill + order age; ingest not wired    |
| **depthflow**   | Loaded / spoof / thinning / neutral           | book, trade   | ❌ unused      | ⚠️ sync.Map         | ⚠️ L2 touch only    | ⚠️ Spoof Trap needs **L3**; top-of-book honest in comment       |
| **causal**      | Alpha / beta / shock / noise                  | trade, ticker | ✅ macro       | ⚠️ sync.Map         | ⚠️ ticker spread    | ⚠️ shock from ticker spread not book void; 442 lines            |
| **liquidity**   | Scarcity / median / robust                    | ticker        | ✅ required    | ✅ none              | n/a                 | ✅ reference for peer-rank pattern                               |
| **sentiment**   | Surge / divergent / slump                     | ticker        | ✅ required    | ✅ none              | n/a                 | ✅ reference for breadth pattern                                 |
| **correlation** | Herd / decoupled / noise / stress             | ticker        | ✅ required    | ✅ none              | n/a                 | ✅ aligned; ⚠️ 0.5 quartile fallback on failure                  |
| **hawkes**      | Frenzy / saturation / organic / exhaust       | trade         | ❌ unused      | ⚠️ sync.Map         | ❌ book dead         | ❌ comment promises book imbalance; book ingest is no-op         |
| **leadlag**     | Lag / sync / decoupled / stall                | ticker        | ❌ own Section | ⚠️ Section sync.Map | n/a                 | ⚠️ works via private Section; not tree-backed                   |
| **exhaust**     | Collapse / thermal / fragile / reversal       | book, trade   | ❌ unused      | ⚠️ sync.Map         | ⚠️ inline L2        | ⚠️ thinning/flip benefit from **L3** delete vs fill             |
| **cvd**         | Absorption / drive / balance / starvation     | trade         | ❌ unused      | ⚠️ sync.Map         | trade only          | ⚠️ solid tape logic; needs tree-only + optional peer starvation |

**Systemic gaps (core signals):** six packages use `sync.Map` as primary history; seven accept `crossSection` but ignore it; three comment blocks promise book/trade data the code never reads.

**Reference implementations to copy:** pumpdump (tree + enrichment), liquidity/sentiment/correlation (crossSection).

### Field signals — deferred (Jun 2025)

Separate function from core signals. **No Track B work until scheduled.**

| Signal        | Role (high level)                          | Notes                                                                                                                 |
|---------------|--------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| **manifold**  | 3D field features → herd/shock/drift/noise | Heavy local `Field` solver; in `trader/signal.go` but field infrastructure, not a microstructure scorer like pumpdump |
| **resonance** | 12-channel sensory vector + batch surprise | Not on `market.Signal`; parallel ingest path (`ObserveIngest`); autoencoder/latent layer                              |

Audit and architecture for these two: **later, dedicated discussion.**

---

### Per-signal: what the comment block still needs

Short list of what each package requires for a **principled** implementation (comment = spec).

- **pumpdump** — leg anchors, exhaustion stamp, wall/imbalance (**L3** wall churn), dollar-volume disqualifier, Kraken multi-leg replay test.
- **fluid** — seek book + trade at score time; **L3** for replenishment/cancel rates; peer activity via crossSection; split file.
- **toxicity** — tree-only; **L3 ingest** + seek at score time; cancel/fill from order delete correlated with trade tape; order age at near-touch for bluff; drop sync.Map.
- **depthflow** — tree-only pressure history; **L3** for spoof shape; tree-seek trades in window when scoring book.
- **causal** — tree-only; book spread for liquidity shock; keep crossSection macro on trades.
- **liquidity** — done; add Kraken replay fixture for rank stability.
- **sentiment** — done; add sector-pump scenario (UNFI+TITCOIN board lit).
- **correlation** — remove 0.5 quartile fallback; fail or derive from peer count.
- **hawkes** — wire **L3** book imbalance into asymmetry OR fix comment; tree-only; drop dead book test path.
- **leadlag** — tree-backed anchor samples OR document Section as intentional index.
- **exhaust** — tree-only; align pressure metric with comment; **L3** for per-level thinning; crossSection for sector-wide exhaustion.
- **cvd** — tree-only; peer volume rank for starvation vs idiosyncratic drive.

*(manifold and resonance: deferred — see Track D.)*

---

### Signal audit checklist (apply to every package)

For each `signal/*/signal.go` comment block:

1. **Read the story** — what chart phase does each category describe?
2. **List data sources** — ticker alone is rarely sufficient for microstructure signals.
3. **Trace `Measure`** — does it seek tree book/trade/**level3** and require `crossSection` where the story needs peers?
4. **Derive every threshold** — name the `statutil` / cross-section function for each gate.
5. **Check replay payload** — can the next frame rebuild baselines from tree measurements only?
6. **Test multi-leg** — at least one scenario with regime transitions (not a single spike).
7. **File size** — split before 400 lines; `classify` as package function when it uses no receiver state.

### Pumpdump current status (Jun 2025)

Refactored: tree-only history, book/trade enrichment, required crossSection (`signal.go`, `history.go`, `classifier.go`, `enrichment.go`). **Still missing:** leg-aware anchors, exhaustion-driven regime reset, book wall/imbalance, dollar-volume gating for thin-book traps, multi-leg test from Kraken OHLC+Trades replay (UNFI/SRM/SLX session shape).

---

### Signal Integrity and Dynamic Calculations

Hardcoded thresholds, static multipliers, or guessed parameters are not permitted. All logic must dynamically adjust to real-time market data.

#### Incorrect (Magic Numbers)

```go
// This uses an arbitrary, hardcoded percentage threshold
func (signalCalculator *SignalCalculator) IsSignalTriggered() bool {
    threshold := 0.015 
    return (signalCalculator.CurrentPrice - signalCalculator.EntryPrice) / signalCalculator.EntryPrice > threshold
}
```

#### Correct (Dynamically Derived)

```go
// This derives the threshold dynamically using Average True Range (ATR) to adjust to market volatility
func (signalCalculator *SignalCalculator) IsSignalTriggered(averageTrueRange float64) bool {
    if signalCalculator.EntryPrice == 0 {
        return false
    }

    volatilityMultiplier := averageTrueRange / signalCalculator.EntryPrice
    percentageChange := (signalCalculator.CurrentPrice - signalCalculator.EntryPrice) / signalCalculator.EntryPrice

    return percentageChange > volatilityMultiplier
}
```

---

## Definition of Done & Verification

Work is complete only when verified. You must provide proof of execution in your completion message.

* **Automated Tests:** Corresponding test coverage must exist, run, and pass for the exact code path changed.
* **Benchmarks:** A performance benchmark must exist and be executed for any data-processing or signal-calculation changes.
* **Verification Output:** You must paste the literal, unmodified stdout output of the test and benchmark runs in your response.

### Preventative Rules:

* **No Fabrication:** If tool or environment limitations prevent you from executing tests or benchmarks, state: `VERIFICATION LIMITATION: UNABLE TO RUN TESTS` and list the exact terminal commands you would run. Do not write mock or simulated test results.
* **Failing Tests:** If tests fail, you must stop and fix the code. Do not proceed or mark a task complete if any suite is failing.

---

## Code Style & Architecture

### Structure

Prefer methods over functions. Compose types to represent logical units.

#### Go Structural Pattern

```go
package packagename

/*
ObjectName manages specialized domain logic.
It handles state updates for our trade calculations.
*/
type ObjectName struct {
    ctx    context.Context
    cancel context.CancelFunc
    err    error
}

/*
NewObjectName instantiates a new ObjectName with a canceled context.
*/
func NewObjectName(ctx context.Context) *ObjectName {
    ctx, cancel := context.WithCancel(ctx)

    return &ObjectName{
        ctx:    ctx,
        cancel: cancel,
    }
}

/*
MethodName performs a state operation.
*/
func (objectName *ObjectName) MethodName() {
    return
}
```

#### TypeScript Structural Pattern
* Use `const` arrow functions rather than standard function declarations.
* Use designated system flex, grid, and typography components instead of standard HTML equivalents.

```tsx
export const PaperEditorApp = () => {
	return (
		<PaperEditorProvider>
			<PaperContextSnapshot />

			<DragDropProvider>
				<Flex.Column className="box-border min-h-0 bg-background" fullHeight>
					<LatexToolbar />

					<Flex.Column className="min-h-0 flex-1" fullHeight>
						<WritingCanvas />
					</Flex.Column>
				</Flex.Column>
			</DragDropProvider>
		</PaperEditorProvider>
	);
};
```

### Size Limits

* **File Size:** Target 200 lines; hard ceiling of 400 lines. Split files exceeding 400 lines into separate types/files.
* **Method Size:** Target under 30 lines. Methods exceeding 60 lines must be split into sub-methods, unless the operation is atomic (e.g., assembly kernels).
* **Type Size:** Limit types to a maximum of 10 methods.

This does *not* mean just move some methods to a new file and call it done. What this means is find the additional responsibilities that the object (type) is doing and compose those onto the current type as a new type. So take the example code above as the type that is over the line count, and do something like:

```go
/*
ObjectName is something descriptive.
It also has a reason why it was implemented.
*/
type ObjectName struct {
    ctx      context.Context
    cancel   context.CancelFunc
    err      error
    composed ComposedObject
}

/*
NewObjectName instantiates a new ObjectName.
It also has a reason for being instantiated.
*/
func NewObjectName(ctx context.Context) (*ObjectName, error) {
    ctx, cancel := ctx.WithCancel(ctx)

    obj := &ObjectName{
        ctx:      ctx,
        cancel:   cancel,
        composed: NewComposedObject(ctx)
    }

    return obj, errnie.Require(map[string]any {
        "ctx":    obj.ctx,
        "cancel": obj.cancel,
    })
}
```

You should recognize objects that do too much when you have naming that is longer than two segments in either method names or object names.

```go
/*
MethodName.
*/
func (objectName *ObjectName) updateSomethingUnrelated() {
    return
}
```

Something like that is usually a good indicator that things are doing to much. In general you want to have one or two segments in names max. Above the ObjectName type is updating something that isn't itself.

```go
/*
MethodName.
*/
func (objectName *ObjectName) update() {
    return
}
```

Now ObjectName is clearly updating itself.

### Control Flow

* **Early Returns:** Write guard clauses with early returns. Keep the primary logic path at indentation level 1.
* **No Else Blocks:** Do not use `else`. Invert conditions to return early or exit.
* **Nesting Ceiling:** Do not nest `if` blocks deeper than two levels. Extract deeply nested logic into a helper method.
* **No Silent Failures:** If a precondition fails or an unexpected state occurs, return a descriptive error. Substituting default fallbacks or silently skipping errors is prohibited.

### Naming & Formatting

* **No Single-Character Names:** Variable names and method receivers must be descriptive (e.g., use `signalCalculator`, not `s`), the exception here is the `testing.TB` instance variable which should always be `t`.
* **Block Separation:** Insert an empty newline between distinct logical code blocks, except where there are only a few lines lines in a block or method/function.
* **Line Breaks:** Wrap long function signatures to prevent lines from running past split-view boundaries.
* **Errors** Instance variables for errors are always `err` and nothing else. Errors are logged with `errnie`

```go
errnie.Error(errnie.Err(
    errnie.Validation, // Not the default, use the correct errnie.Kind
    "some message",    // or err.Error()
    err,
))
```
---

## Environment & Tooling Constraints

### Git State Integrity

* Do not read, query, or reference git history, commit logs, or previous branches to solve bugs. Base your solution entirely on the current state of the codebase. The answer/solution rarely lies in the past.
* Never run `git checkout`, `git reset`, `git restore`, or any command that discards working tree changes. If a revert is required, stop and request user intervention.

### Compiler Configuration & Linker Errors

* **dropg Linker Error:** If you encounter a `dropg` linker error, refer to the `Makefile` located in the project root to ensure environment flags and compiler options match the project targets. Do not bypass build constraints with temporary flags.

---

## Interaction Protocol

1. **No Summarization:** Do not explain the existing system architecture back to the user. Reference specific file names and types when discussing changes.
2. **Opinions on Request Only:** Provide design opinions or alternative paradigms only when explicitly asked. Otherwise, implement the requested change directly according to this contract.
3. **Preserve Load-Bearing Structure:** Read and trace existing code paths before proposing modifications. Do not rewrite structural components unless you can document exactly why the existing implementation is broken or incorrect.
4. Keep your answers brief. The user cannot process language like you do, and requires your answer to roughly match their own levels of verbosity.

---

## Signal, Artifact, and Measurement Composition

This section records the canonical architecture. If a task requires wiring beyond what is described here, the gap is in **ingestion** (artifact not written to the tree with the right prefix) or in the **signal Measure implementation**, not in trader fan-out or nomagique transport glue.

> **Current scoring path:** signals score inline in Go from tree ingest. Pure `nomagique.Number` + `transport.NewFlipFlop` pipelines are not the production path.

### One tree, write at the source, query everywhere else

Market data enters the system once: **websockets write directly to `dmt.Tree`**.

```go
tree.Insert(artifact.Prefix(), artifact.Marshal())
```

`kraken/public/websocket.go` (and private/user websockets) acquire an artifact from raw Kraken JSON, set Role/Scope/Origin, and insert. No trader fan-out, no per-signal `Update`, no intermediate book/trade/ticker types relaying the same frame.

**Traders and signals do not ingest.** They **query** what they need by prefix and score in Go:

```go
for artifact := range tree.Seek(query.Prefix()) {
    measured := signal.Measure(artifact)
    // emit measurement artifact with output/confidence/surprise/strength
}
```

Do not reproduce the orchestration in `trader/crypto.go` — wiring every channel through `book.Update` → `updateSignals` → `signal.Update` is redundant once the tree is the bus. That layer exists to be removed, not extended.

### The signal contract

A signal has one job: **Measure** — seek the tree by declared ingest roles, update internal state from raw artifacts, return measurement artifacts with dynamically derived `output` fields.

Reference implementations: `signal/toxicity/signal.go`, `signal/pumpdump/signal.go`, `signal/fluid/signal.go`.

Do not add tracker access, category switches, feature encoding, or ingestion inside `Measure` beyond what the signal needs to score the incoming artifact batch. Windows grow from observed timestamps via `statutil.WindowDepth`; do not gate on warmup sample counts or fixed horizons.

### Inline Go scoring, not nomagique pipelines

Domain scoring lives in Go on the signal type:

* ingest roles declare which tree prefixes the signal replays (`IngestRoles()`)
* `Measure(*datura.Artifact, *CrossSection)` scores from tree queries and cross-section peers; it does **not** maintain local per-pair state — prior measurements in the tree are the replay source
* thresholds, windows, and category labels are derived from live market statistics (`statutil`, cross-section snapshots, peer windows)
* measurement payloads expose `output.confidence`, `output.surprise`, `output.strength`, `output.elapsed`, and category indices consumed by `logic` playbook walks

`nomagique` remains available for reusable math primitives where they already fit, but **do not** block signal work on composing a full `nomagique.Number` pipeline or `transport.NewFlipFlop` graph.

### datura.Artifact: payload, attributes, prefix

| Field                     | Role                                                                                                 |
|---------------------------|------------------------------------------------------------------------------------------------------|
| **Payload**               | The data — usually JSON (raw Kraken book/trade/order events)                                         |
| **Type**                  | Describes payload encoding (`json`, `artifacts`, …)                                                  |
| **Role / Scope / Origin** | Semantic indexing; together they determine **Prefix**                                                |
| **Attributes**            | Schema for the payload — key names, types, relationships, extraction rules. Not a second data store. |

Do not abuse attributes for operational data. Put market data in the payload; describe how to read and process it in attributes.

**Attributes are the configuration surface.** Conventions are chosen and kept consistent across the codebase — they are not fixed by capnp schema. A primitive reads attributes at `Read` time to decide how to handle payload fields. This is why `datura.Artifact` exists: rigid Go structs cannot express per-field transforms, optional pipelines, or evolving schemas without constant type churn. The trade-off is slightly higher risk of typos in attribute keys; the gain is a system that adapts without recompilation.

#### Per-field transforms

When one payload key needs EMA and another needs raw value, do not fork the pipeline or add signal glue. Declare it on the schema artifact:

```go
artifact.WithAttributes(datura.Map{
    "keys": datura.Map{
        "cancelBid": "float",
        "fillBid":   "float",
    },
    "transforms": datura.Map{
        "cancelBid": "ema",
        "fillBid":   "raw",
    },
})
```

The nomagique primitive reads `transforms.<payloadKey>` and applies the matching stage (`adaptive.NewEMA`, pass-through, etc.). Same extractor, different behavior per key — driven by attributes, not constructor parameters.

#### How far attributes can go

In principle, attributes can describe almost anything a Go type would:

* **Schema** — key names, types, units, relationships between fields
* **Transforms** — ema, zscore, fracdiff, per key or per path
* **Rules** — thresholds, gates, priority order (could mirror `nomagique/logic` in attribute form)

Replicating entire `nomagique/logic` circuits purely in attributes is possible but not recommended in practice — use `logic.NewCircuit` in the pipeline for branching, attributes for field-level config. Prefer composition in Go where the graph is stable; use attributes where the graph varies by signal, scope, or instrument without new types.

#### When pipeline composition is not enough

If a value cannot be wrapped cleanly in `nomagique.Number(...)`:

1. First — can an attribute convention express it? (transform, gate source, aggregation window)
2. Second — does a nomagique primitive need to grow to honor that attribute?
3. Last — only then consider `datura/transport` (Graph, Feedback) for routing

Never add a new Go struct or signal method when an attribute on the schema artifact would do.

**Prefix** is the tree query API. `Artifact.Prefix()` builds `role/scope/origin/.../timestamp/uuid.type`.

Example — Origin `toxicity`, Role `measurement`, Scope `book`:

```
measurement/book/toxicity.<type>
```

Consumers seek by prefix:

* `book` — all book events (raw Kraken feed)
* `measurement` — all measurements across signals
* `measurement/book` — book measurements from every signal that emits them

Ingestion prefixes describe **what arrived** (e.g. Role `book`, Scope `BTC/USD`). Measurement prefixes describe **what was derived** (e.g. Role `measurement`, Scope `book`, Origin `toxicity`). Same tree, different queries.

Plug raw Kraken JSON into the payload at ingest time. Signals parse payload fields directly in Go — not pre-serialized float batches through nomagique extractors.

### Incorrect vs correct

#### Incorrect — trader orchestrates ingest, signal has Update, nomagique FlipFlop scoring

```go
// crypto.go: websocket → book.Update → updateSignals → toxicity.Update → tree
signal.Update(artifact) // redundant relay
for artifact := range tree.Seek(measurementQuery.Prefix()) {
    transport.NewFlipFlop(&artifact, nomagique.Number(...))
}
```

#### Correct — websocket writes once, trader replays ingest, signal scores inline

```go
// kraken/public/websocket.go — on book frame:
artifact := datura.Acquire("kraken", datura.APPJSON).
    WithRole("book").WithScope(symbol).WithPayload(rawJSON)
tree.Insert(artifact.Prefix(), artifact.Marshal())

// trader/signal.go — replay unseen ingest by role, call each signal's Measure:
measured := binding.signal.Measure(artifact)
measured.WithRole("measurement")
measured.SetOrigin("toxicity")
crypto.insertMeasurement(measured)
```

If extra wiring is needed beyond **websocket → tree → trader.Signal.Measure → UI**, stop and fix ingest prefixes, measurement payload shape, or the signal's Go scoring — do not grow trader relay layers or nomagique transport graphs.

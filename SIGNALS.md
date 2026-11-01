# signals

The grid reads **Action** tokens and **Oracle** tokens. Brightness of each
token comes from the metrics pinned below. Downstream selection is
`Oracle × Action` strength — oracles choose whether a name is tradable /
worthy; actions choose which story phase is lit.

| Kind   | Tokens            | Channels                      | Regions |
|--------|-------------------|-------------------------------|---------|
| Action | Enter, Hold, Exit | Detector, Support, Opposition | R00–R08 |
| Oracle | listed below      | Veto, Approve                 | R09–R22 |

Region layout (23 total): Enter 0–2, Exit 3–5, Hold 6–8; then each
oracle as Veto/Approve pairs — Volume Quote 9–10, Day Move 11–12, Spread
13–14, Book Integrity 15–16, Board Confidence 17–18, Contagion 19–20,
Capacity 21–22.

Sources:

- `envelope.*` — fields on `runtime.Envelope` after each signal `Step`
- `broker.Price` / `broker.Mover` / `broker.Tape` / `broker.Instrument` —
  desk-side readings that are not (yet) envelope layers

Wire form in tables: `source/Field` (envelope) or `broker.X/Method`.

---

## Action — Enter

Story: open a lot.

| Channel    | Metric                       | Source                 |
|------------|------------------------------|------------------------|
| Detector   | `cvd/Drive`                  | `envelope.CVD`         |
| Detector   | `cvd/NetFraction`            | `envelope.CVD`         |
| Detector   | `pumpdump/Compression`       | `envelope.Pumpdump`    |
| Detector   | `pumpdump/Precursor`         | `envelope.Pumpdump`    |
| Support    | `correlation/HerdScore`      | `envelope.Correlation` |
| Support    | `correlation/Strength`       | `envelope.Correlation` |
| Support    | `leadlag/InefficientScore`   | `envelope.LeadLag`     |
| Support    | `leadlag/SyncScore`          | `envelope.LeadLag`     |
| Support    | `toxicity/SupportScore`      | `envelope.Toxicity`    |
| Opposition | `cvd/Starvation`             | `envelope.CVD`         |
| Opposition | `pumpdump/RVOL` (thin tape)  | `envelope.Pumpdump`    |
| Opposition | `depthflow/LoadedScore`      | `envelope.DepthFlow`   |
| Opposition | `morphology/BidNotionalLean` | `envelope.Morphology`  |

---

## Action — Hold

Story: stay in an open lot.

| Channel    | Metric                        | Source                |
|------------|-------------------------------|-----------------------|
| Detector   | `hawkes/Fit.SpectralRadius`   | `envelope.Hawkes`     |
| Detector   | `hawkes/TotalBuyDescendants`  | `envelope.Hawkes`     |
| Detector   | `hawkes/TotalSellDescendants` | `envelope.Hawkes`     |
| Detector   | `pumpdump/Ignition`           | `envelope.Pumpdump`   |
| Support    | `depthflow/LoadedScore`       | `envelope.DepthFlow`  |
| Support    | `morphology/ShapeStable`      | `envelope.Morphology` |
| Support    | `toxicity/SupportScore`       | `envelope.Toxicity`   |
| Support    | `sentiment/SurgeScore`        | `envelope.Sentiment`  |
| Support    | `sentiment/Breadth`           | `envelope.Sentiment`  |
| Opposition | `pumpdump/Exhaustion`         | `envelope.Pumpdump`   |
| Opposition | `liquidity/ScarcityScore`     | `envelope.Liquidity`  |
| Opposition | `depthflow/ThinScore`         | `envelope.DepthFlow`  |
| Opposition | `sentiment/SlumpScore`        | `envelope.Sentiment`  |

---

## Action — Exit

Story: leave an open lot.

| Channel    | Metric                         | Source                |
|------------|--------------------------------|-----------------------|
| Detector   | `cvd/Absorption`               | `envelope.CVD`        |
| Detector   | `cvd/Net` (sign flip vs prior) | `envelope.CVD`        |
| Detector   | `pumpdump/Exhaustion`          | `envelope.Pumpdump`   |
| Support    | `depthflow/ThinScore`          | `envelope.DepthFlow`  |
| Support    | `toxicity/BluffScore`          | `envelope.Toxicity`   |
| Support    | `toxicity/VacuumScore`         | `envelope.Toxicity`   |
| Support    | `depthflow/SpoofScore`         | `envelope.DepthFlow`  |
| Opposition | `depthflow/LoadedScore`        | `envelope.DepthFlow`  |
| Opposition | `morphology/AskVacuum`         | `envelope.Morphology` |
| Opposition | `pumpdump/Trend`               | `envelope.Pumpdump`   |

---

## Oracle — 24h Volume Quote

Universe filter: enough quote turnover to bother.

| Side    | Metric                | Source                                                        |
|---------|-----------------------|---------------------------------------------------------------|
| Veto    | volume below floor    | `broker.Price.Mover/VolumeUSD` vs `risk.min_mover_volume_usd` |
| Approve | volume at/above floor | `broker.Price.Mover/VolumeUSD`                                |
| Approve | instrument is known   | `broker.Instrument/Has`                                       |

---

## Oracle — Day Move

Liquid-but-flat names stay out.

| Side    | Metric                      | Source                                                        |
|---------|-----------------------------|---------------------------------------------------------------|
| Veto    | open-to-last below floor    | `broker.Price.Mover/ChangePct` vs `risk.min_mover_change_pct` |
| Approve | open-to-last at/above floor | `broker.Price.Mover/ChangePct`                                |

---

## Oracle — Spread

Executable width must clear entry and stay tradeable.

| Side    | Metric                            | Source                                                  |
|---------|-----------------------------------|---------------------------------------------------------|
| Veto    | touch spread above cap            | `broker.Price/SpreadPct` vs `risk.max_entry_spread_pct` |
| Veto    | `liquidity/ScarcityScore` extreme | `envelope.Liquidity`                                    |
| Approve | tight touch vs board              | `broker.Price/SpreadPct`                                |
| Approve | `liquidity/MedianScore`           | `envelope.Liquidity`                                    |
| Approve | `pumpdump/Spread` (compressed)    | `envelope.Pumpdump`                                     |

---

## Oracle — Book Integrity

Desk-lot physics: cash, backing, tape, open drag.

| Side    | Metric                    | Source                                                           |
|---------|---------------------------|------------------------------------------------------------------|
| Veto    | top-bid cash below floor  | `broker.Price/TouchBidCash` vs `risk.min_touch_bid_cash`         |
| Veto    | backing ratio below floor | `broker.Price/BackingRatio` vs `risk.min_backing_ratio`          |
| Veto    | tape not alive            | `broker.Tape/Alive` (`risk.tape_window`, `risk.min_tape_trades`) |
| Veto    | close-now drag beyond cap | `broker.Price/EntryCloseNowPnLPct` vs open-drag budget           |
| Approve | backing ratio firm        | `broker.Price/BackingRatio`                                      |
| Approve | `toxicity/SupportScore`   | `envelope.Toxicity`                                              |
| Approve | `liquidity/DepthScore`    | `envelope.Liquidity`                                             |

---

## Oracle — Board Confidence

Cross-symbol approve that the board story matches the action phase.

| Side    | Metric                              | Source                                |
|---------|-------------------------------------|---------------------------------------|
| Approve | `correlation/PeakScore`             | `envelope.Correlation` (board median) |
| Approve | `correlation/HerdScore`             | `envelope.Correlation` (board median) |
| Approve | `pumpdump/Compression`              | `envelope.Pumpdump` (board median)    |
| Approve | `hawkes/Fit.SpectralRadius`         | `envelope.Hawkes` (board median)      |
| Approve | `sentiment/SurgeScore`              | `envelope.Sentiment` (board median)   |
| Approve | `sentiment/Breadth`                 | `envelope.Sentiment` (board median)   |
| Approve | `depthflow/ThinScore` (bid release) | `envelope.DepthFlow` (board median)   |
| Approve | `toxicity/VacuumScore`              | `envelope.Toxicity` (board median)    |
| Veto    | `correlation/StressScore`           | `envelope.Correlation` (board median) |
| Veto    | `sentiment/SlumpScore`              | `envelope.Sentiment` (board median)   |
| Veto    | `pumpdump/Exhaustion`               | `envelope.Pumpdump` (board median)    |

---

## Oracle — Contagion

Lead structure breaking or cohort stress — do not trust the path.

| Side    | Metric                    | Source                 |
|---------|---------------------------|------------------------|
| Veto    | `leadlag/DecoupledScore`  | `envelope.LeadLag`     |
| Veto    | `leadlag/StallScore`      | `envelope.LeadLag`     |
| Veto    | `correlation/StressScore` | `envelope.Correlation` |
| Veto    | `correlation/NoiseScore`  | `envelope.Correlation` |
| Approve | `leadlag/SyncScore`       | `envelope.LeadLag`     |
| Approve | `correlation/HerdScore`   | `envelope.Correlation` |
| Approve | `correlation/AlphaScore`  | `envelope.Correlation` |

---

## Oracle — Capacity

Can the book carry the desk lot without blowing friction / impact.

| Side    | Metric                           | Source                                                      |
|---------|----------------------------------|-------------------------------------------------------------|
| Veto    | `liquidity/ScarcityScore`        | `envelope.Liquidity`                                        |
| Veto    | `toxicity/VacuumScore`           | `envelope.Toxicity`                                         |
| Veto    | `depthflow/ThinScore`            | `envelope.DepthFlow`                                        |
| Veto    | open-drag / exit impact too high | `broker.Price/EntryCloseNowPnLPct`, `broker.Price/Friction` |
| Approve | `liquidity/DepthScore`           | `envelope.Liquidity`                                        |
| Approve | `liquidity/MedianScore`          | `envelope.Liquidity`                                        |
| Approve | `toxicity/SupportScore`          | `envelope.Toxicity`                                         |

---

## Envelope inventory

Every scored field on `runtime.Envelope` and where it pins. Metadata
(`Category`, `Ready`, `Eligible`, timestamps, raw `Price`, `Fit` params
used only as intermediates) is omitted unless it is itself a score.

### `envelope.Hawkes` → `excitation.Outcome`

| Field                             | Pins                                        |
|-----------------------------------|---------------------------------------------|
| `Fit.SpectralRadius`              | Hold Detector; Board Confidence Approve     |
| `ImmediateBuyOffspring`           | Hold Detector (asymmetry raw)               |
| `ImmediateSellOffspring`          | Hold Detector (asymmetry raw)               |
| `TotalBuyDescendants`             | Hold Detector                               |
| `TotalSellDescendants`            | Hold Detector                               |
| `BuyArrivalRate`                  | Hold Support                                |
| `SellArrivalRate`                 | Hold Opposition                             |
| `Maturity`                        | Board Confidence Approve (readiness weight) |
| `HawkesPoissonLogLikelihoodDelta` | — (diagnostics)                             |
| `CrossSelfLogLikelihoodDelta`     | — (diagnostics)                             |
| `EventCount` / side counts        | — (diagnostics)                             |

### `envelope.Correlation` → `equation.CohortOutput`

| Field                                 | Pins                                                       |
|---------------------------------------|------------------------------------------------------------|
| `HerdScore`                           | Enter Support; Contagion Approve; Board Confidence Approve |
| `AlphaScore`                          | Contagion Approve                                          |
| `NoiseScore`                          | Contagion Veto                                             |
| `StressScore`                         | Contagion Veto; Board Confidence Veto                      |
| `PeakScore`                           | Board Confidence Approve                                   |
| `Correlation` / `Energy` / `Strength` | Enter Support (strength weight)                            |

### `envelope.CVD` → `equation.FlowOutput`

| Field         | Pins                      |
|---------------|---------------------------|
| `Drive`       | Enter Detector            |
| `Absorption`  | Exit Detector             |
| `Balance`     | Enter Support             |
| `Starvation`  | Enter Opposition          |
| `Net`         | Exit Detector (sign flip) |
| `NetFraction` | Enter Detector            |
| `Value`       | strength rollup           |

### `envelope.DepthFlow` → `equation.BookflowOutput`

| Field                  | Pins                                                                   |
|------------------------|------------------------------------------------------------------------|
| `LoadedScore`          | Enter Opposition; Hold Support; Exit Opposition                        |
| `SpoofScore`           | Exit Support                                                           |
| `ThinScore`            | Hold Opposition; Exit Support; Capacity Veto; Board Confidence Approve |
| `NeutralScore`         | — (idle book)                                                          |
| `HypothesisSeparation` | Hold Detector (confidence weight)                                      |
| `Value`                | strength rollup                                                        |

### `envelope.LeadLag` → `algorithm.LagOutcome`

| Field              | Pins                             |
|--------------------|----------------------------------|
| `InefficientScore` | Enter Support                    |
| `SyncScore`        | Enter Support; Contagion Approve |
| `DecoupledScore`   | Contagion Veto                   |
| `StallScore`       | Contagion Veto                   |
| `Strength`         | strength rollup                  |

### `envelope.Pumpdump` → `equation.IgnitionOutput`

| Field                | Pins                                                  |
|----------------------|-------------------------------------------------------|
| `Compression`        | Enter Detector; Board Confidence Approve              |
| `Precursor`          | Enter Detector                                        |
| `Ignition`           | Hold Detector                                         |
| `RVOL`               | Enter Opposition (starved when low)                   |
| `Spread`             | Spread Approve (compression)                          |
| `Trend`              | Exit Opposition                                       |
| `Exhaustion`         | Hold Opposition; Exit Detector; Board Confidence Veto |
| `Buy.*` / `Sell.*`   | same families, directional                            |
| `Value` / `Strength` | strength rollup                                       |

### `envelope.Sentiment` → `equation.ConvictionOutput`

| Field                | Pins                                   |
|----------------------|----------------------------------------|
| `SurgeScore`         | Hold Support; Board Confidence Approve |
| `DivergentScore`     | Contagion Veto (idiosyncratic)         |
| `SlumpScore`         | Hold Opposition; Board Confidence Veto |
| `Breadth`            | Hold Support; Board Confidence Approve |
| `Change`             | Day Move complement (tick-scale)       |
| `Value` / `Strength` | strength rollup                        |

### `envelope.Liquidity` → `equation.DepthOutput`

| Field                | Pins                                        |
|----------------------|---------------------------------------------|
| `ScarcityScore`      | Hold Opposition; Spread Veto; Capacity Veto |
| `MedianScore`        | Spread Approve; Capacity Approve            |
| `DepthScore`         | Book Integrity Approve; Capacity Approve    |
| `Value` / `Strength` | strength rollup                             |

### `envelope.Toxicity` → `equation.BookQualityOutput`

| Field                | Pins                                                                  |
|----------------------|-----------------------------------------------------------------------|
| `BluffScore`         | Exit Support                                                          |
| `VacuumScore`        | Exit Support; Capacity Veto; Board Confidence Approve                 |
| `SupportScore`       | Enter Support; Hold Support; Book Integrity Approve; Capacity Approve |
| `Value` / `Strength` | strength rollup                                                       |

### `envelope.Morphology` → `equation.MorphologyOutput`

| Field                | Pins             |
|----------------------|------------------|
| `ShapeStable`        | Hold Support     |
| `AskVacuum`          | Exit Opposition  |
| `BidNotionalLean`    | Enter Opposition |
| `Value` / `Strength` | strength rollup  |

### Broker (not on envelope)

| Reading                            | Pins                     |
|------------------------------------|--------------------------|
| `broker.Price.Mover/VolumeUSD`     | 24h Volume Quote         |
| `broker.Price.Mover/ChangePct`     | Day Move                 |
| `broker.Instrument/Has`            | 24h Volume Quote Approve |
| `broker.Price/SpreadPct`           | Spread                   |
| `broker.Price/Friction`            | Capacity Veto            |
| `broker.Price/TouchBidCash`        | Book Integrity           |
| `broker.Price/BackingRatio`        | Book Integrity           |
| `broker.Tape/Alive`                | Book Integrity           |
| `broker.Price/EntryCloseNowPnLPct` | Book Integrity; Capacity |
| `broker.Price/OpenDragBudgetPct`   | Book Integrity cap       |

---

## Notes

- Board Confidence metrics marked *(board median)* are the same envelope
  fields, aggregated across symbols on the tick before pinning — same role
  as the old `…/aggregate/…` layers.
- A metric may pin to more than one token; brightness still divides by the
  token's channel width, not by how many pins lit.
- `grid_pins.go` is the machine-readable mirror of these tables; keep them
  in sync when either moves.
- Emit policy (`Grid.Emit`): flat books publish Enter + oracles; open lots
  publish Hold/Exit + oracles. Each oracle keeps only `max(Veto, Approve)`.
  Support/Opposition stay lit — `Tree.Predict` resolves them against path
  resistance.
- Path tokens are dotted compounds: lit oracle winners in index order, then
  the brightest action channel — e.g. `R10.R12.R14.R16.R18.R00`. `/` joins
  successive compounds in the sequencer cut.
- Volume / Day Move / Spread / Book Integrity are painted from `broker.Price`
  (mover + book) before Emit; they are not envelope Map() labels.

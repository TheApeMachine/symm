# `signal/exhaust` — is the microstructure that supported this move decaying?

> A position doesn't need the market to reverse to stop being safe. It just
> needs the depth, spread, and pressure that made the move look real to
> quietly stop agreeing with it.

## What this package is

Exhaust measures **microstructure decay against a held position** — not
"is the market reversing" but "are the specific conditions (depth, spread,
trade pressure, book imbalance) that would corroborate this position still
present, or are they thinning out." It scores **both hypothetical sides**
(long-exit and short-exit) on every observation and leaves the caller to pick
the side matching its actual position — the doc comment on `Signal` states
this directly: "Position inventory is deliberately absent."

This package is the market adapter around
[`nomagique/algorithm.DecaySample`](../../../nomagique/algorithm/decay_sample.go)
(book/trade ingestion into the four raw decay ingredients) and
[`nomagique/equation.Decay`](../../../nomagique/equation/decay.go)
(classification and fusion). No position sizing, no trading gate —
`logic/category` and the strategy layer decide what a reading means for an
actual open position.

## Four independent decay families, one per side

For each side (long/short), `Decay.exitSide` scores four distinct
microstructure failure modes, each derived from the current observation
against **that same series' own accumulated history** (a ratio or standardized
deviation, never an absolute threshold):

| Family                                  | What it measures                                                                                         | Formula (informal)                                                                                                                                                   |
|-----------------------------------------|----------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Mechanical** (thinning/collapse)      | Is the depth that would support this side, or the overall book density, below its own observed baseline? | `max(ratioDecline(sideDepthRatio), ratioDecline(densityRatio))` — `ratioDecline(r) = max(0, 1-r)`, zero once the ratio is at or above its own mean.                  |
| **Fragile** (spread widening)           | Is the current spread wider than its own historical norm?                                                | `max(0, spreadDeviation)` — a standardized deviation, shared identically by both sides (spread widening is not directional).                                         |
| **Thermal** (pressure fade + rejection) | Has trade pressure faded from its own running extreme *and* has price actually moved against this side?  | `pressureFade(pressure, extremum, side) × priceRejection(priceReturn, side)` — a product, so fading pressure with no adverse price move (or vice versa) scores zero. |
| **Reversal** (imbalance flip)           | Has book imbalance flipped from favoring this side to favoring the other?                                | `imbalanceFlip` — zero unless the *prior mean* imbalance actually favored this side and the *current* reading has crossed to favor the other.                        |

The asymmetry in Thermal is deliberate and load-bearing: `pressureFade` alone
cannot establish exhaustion, because pressure naturally rises and falls.
Requiring the held side's price to have *also* rejected — moved against
it — is what turns "pressure went down" into "pressure went down and the
market confirmed it by moving against this position." A long can only be
thermally exhausted by a negative price return; a short only by a positive
one (`priceRejection`).

Pressure fade compares against the running **peak** (long) or **trough**
(short) *including the current tick* — a tick that sets a new extreme is, by
construction, not fading from anything yet (it *is* the new extreme), so it
reports zero fade rather than a spurious negative reading.

## Fusion: softmax-weighted, not max

Unlike most other signals in this codebase (which take a flat `max` across
families), Exhaust fuses its four margins with a **softmax** over their own
values (`probability.SoftmaxScoresNormalized`), then takes the
probability-weighted average as `Urgency`:

```
weights  = softmax(mechanical, fragile, thermal, reversal)
urgency  = Σ weight × margin
```

This means `Urgency` is not simply "whichever family is strongest" — it's a
soft blend that gives more credit to a reading where multiple families agree
(several elevated margins) than one where a single family dominates and the
rest are near zero, even if the single dominant margin in the second case is
numerically larger. `Category` (the classification label: 1=mechanical,
2=fragile, 3=thermal, 4=reversal, 0=none) is still assigned by simple `max` —
fusion changes the *urgency score*, not which family gets named as the
winning story.

## Every metric this package produces

All published under `source=exhaust`, sided `buy` (evidence for exiting a
**long**) and `sell` (evidence for exiting a **short**) — this is a
deliberate reuse of the buy/sell side vocabulary to mean "which position this
evidence argues for closing," not "which direction just traded."

| Metric       | Meaning                                                                                                                                                                                                                                                                                                                                                             |
|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `mechanical` | `max(depth-ratio decline, density-ratio decline)` margin for this side — is the book itself (not spread/pressure/imbalance) below its own baseline. `0..1`.                                                                                                                                                                                                         |
| `fragile`    | Spread-widening margin — identical value on both sides (not side-specific). `0..1`.                                                                                                                                                                                                                                                                                 |
| `thermal`    | Pressure-fade × price-rejection margin — requires both fading pressure and adverse price movement for this side. `0..1`.                                                                                                                                                                                                                                            |
| `reversal`   | Imbalance-flip margin — requires the book to have actually crossed from favoring this side to favoring the other. `0..1`.                                                                                                                                                                                                                                           |
| `urgency`    | Softmax-weighted fusion of all four margins — see above. `0..1`.                                                                                                                                                                                                                                                                                                    |
| `strength`   | Same value as `urgency` (`DecaySideOutput.Value == Strength == Urgency`).                                                                                                                                                                                                                                                                                           |
| `category`   | Nominal winning-family identifier: `0` none, `1` mechanical, `2` fragile, `3` thermal, `4` reversal. **Deliberately not normalized** — the doc comment on `normalizedDecayMetrics` explains why: treating this integer code as a magnitude would invent a false ordering between mechanical, fragile, thermal, and reversal exhaustion. It is a label, not a score. |

### Readiness

All six scalar metrics (`mechanical` through `strength`) require a finite
value in `0..1` (`normalizedDecayScore`) — the equation itself enforces this
bound internally (each family is a `probability.MagnitudeMargin`), so a
`nil` normalized reading here would indicate a computation error, not an
expected runtime state. `category` is validated separately as a finite
integer in `0..4` (`validDecayCategory`) and deliberately carries no
`Normalized` field at all.

## Two observation paths, same measurement shape

Like `signal/depthflow`, this signal ingests both book updates and
individual trades through the same `algorithm.DecaySample`/`equation.Decay`
pipeline and emits the same `Metrics` shape from either
(`frame`) — trades are held until a book observation exists at or before
their timestamp, and unchanged book snapshots are deduplicated
(`sameSnapshot`) before triggering a new measurement, exactly as in
`depthflow`.

## Files

| File        | Responsibility                                                                                          |
|-------------|---------------------------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, book/trade dispatch and deduplication, measurement projection, category validation. |

The decay ingredient extraction (depth ratios, spread deviation, pressure
extrema, imbalance mean) lives in `nomagique/algorithm/decay_sample.go`; the
four-family scoring and softmax fusion live in `nomagique/equation/decay.go`
— not here.

## What this package deliberately does not decide

A high `thermal` reading on the `buy` side is not "close the long now" — it
is "the pressure and price action that would corroborate a long position are
fading and rejecting, right now." Whether that's enough to actually exit —
weighed against fees, the position's own P&L state, and everything else
measured this tick — is the strategy layer's question, not this signal's.

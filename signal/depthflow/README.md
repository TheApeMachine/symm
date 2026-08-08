# `signal/depthflow` — is the book loaded, spoofed, thinning, or just quiet?

> A one-sided book is only informative once you know whether that side is
> real, being tested, or being pulled.

## What this package is

Depthflow is the **"Weight of the Book"** perspective — it measures signed
depth imbalance around the midpoint, at two different resolutions
simultaneously (touch-only and full-depth-with-decay), and classifies the
result against the symbol's own recent imbalance history into one of four
mutually exclusive book states: **loaded** (real, corroborated one-sided
depth), **spoofed** (touch and full-depth disagree in direction — the
signature of a large resting order that isn't at the touch, or a touch order
that contradicts the deeper book), **thinning** (total book notional falling
below its own recent baseline), or **neutral** (none of the above).

This package is the market adapter around
[`nomagique/algorithm/book/flow`](../../../nomagique/algorithm/book/flow)
(book state, imbalance, toxicity) and
[`nomagique/equation.Bookflow`](../../../nomagique/equation/bookflow.go)
(classification). No category, no trading gate — `logic/category` decides
what a reading means.

## Depth is weighted by distance from mid, not counted flat

`Book.Imbalance` does not sum raw resting quantity per side. Each level's
contribution decays with its distance from the midpoint, at a rate
(`DecayRate`) derived from the **relative spread**: `1/(spread/mid)` — a
tight spread produces a fast decay (only depth very near the touch counts
much), a wide spread produces a slow decay (depth further out still
matters). This makes "how much depth counts" scale with the market's own
current tightness rather than an absolute tick distance that would mean
different things at different price levels or volatility regimes.

Two resolutions of the same imbalance are computed every book update:

- **Weighted (`touchOnly=false`)** — full decayed-depth imbalance across the
  whole retained book.
- **Level1 (`touchOnly=true`)** — imbalance using only the best bid/ask
  quantity, no decay.

A third, **Flat** imbalance is computed only when a `FlatDepth` (a
tick-count depth derived from the book's own structure, `Book.FlatDepth`) is
resolvable — an alternate full-depth reading independent of the decay
weighting, used as a second corroborating signal for spoof detection.

## Toxic depth is discounted before it can vote

Before computing imbalance, each side's weight is discounted by
`ToxicPenalty(touchCancel, frameAdd, touchDepth)` — the share of touch
liquidity that was **cancelled and not replaced** within the same atomic book
update (as opposed to liquidity that was cancelled and immediately reposted
at a new price, which contributes no toxicity — see `signal/toxicity` for
the fuller cancellation-vs-retreat distinction this shares a spirit with).
Unreplaced cancellation at the touch actively reduces that side's weight in
the imbalance calculation — a side that is rapidly shedding real resting
liquidity does not get to vote as if that liquidity were still there.

## Trade pressure is a decaying EMA, not a window sum

`window.tradePressure` is not "sum of recent signed trade notional in a
window" — it's an exponential moving average whose decay half-life is itself
derived from the symbol's own **observed inter-trade gap** (`halfLife =
tradeGapSum/tradeGaps`, `alpha = 1 - exp(-ln2 · elapsed/halfLife)`). A
symbol that trades in rapid bursts gets a fast-decaying pressure reading (old
pressure fades quickly); a symbol that trades sparsely gets a slow-decaying
one — the "how recent is recent" question is answered from the symbol's own
tempo, not a fixed lookback.

## Classification: two imbalance readings compared against their own history

`Bookflow.Measure` derives thresholds from **median absolute** historical
values of the weighted and level1 imbalance series (`bookflowMedianAbsolute`)
— an empirical "how extreme is extreme, for this symbol" bar, not a fixed
constant:

| State        | Condition                                                                                                                                                                                                        | Reading                                                                                                                                                                      |
|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Spoofed**  | `\|weighted\|` exceeds its own historical threshold, but weighted and level1 (or flat) disagree in sign, and level1's magnitude is significant relative to a contrast ratio derived from the same two thresholds | The touch and the deeper book are telling different directional stories — classic evidence that the deep-book skew is not backed by genuine touch commitment, or vice versa. |
| **Thinning** | Current total book notional is below the median of its own prior history                                                                                                                                         | The book has less total resting value than it typically does right now, independent of direction.                                                                            |
| **Loaded**   | Not spoofed, not thinning; weighted and level1 agree in sign and both exceed their own historical thresholds                                                                                                     | Both resolutions of imbalance agree, and both are unusually large for this symbol — the "this is real, corroborated pressure" reading.                                       |
| **Neutral**  | None of the above                                                                                                                                                                                                | Nothing distinguishing about the book's current shape.                                                                                                                       |

Spoof and thinning are checked **before** loaded, and loaded is defined as
"not spoofed and not thinning" — so a book that would otherwise look loaded
but has a contradicting touch/depth signal, or is thinning overall, is
never also classified as loaded. Each observation gets exactly one category.

`loadedScore` additionally folds in trade-pressure confirmation
(`bookflowLoadedScore`): the loaded reading is boosted when trade pressure
agrees in sign with the book imbalance (someone is actually trading into the
lopsided side), and damped when trade pressure disagrees — a loaded book with
contradicting trade flow is treated as less convincingly loaded than one the
tape is actively confirming.

## Two observation paths, one measurement shape

This signal ingests two kinds of events per symbol — book updates (L2/L3
book deltas, reconstructed by `websocket.BookSource`) and individual trades —
and both call `signal.frame` to build the same `Metrics` shape, so a consumer
does not need to know which event produced a given reading. Trades are
skipped until a book observation has been seen at or before their timestamp
(`bookAt`/`bookPending` in `signal.go`) — trade pressure needs a book context
to be meaningful, so a trade arriving before any book state is simply held
until the book catches up.

Book updates are deduplicated: `sameSnapshot` compares the current book's
per-tick bid/ask levels against the last retained snapshot, and if nothing
actually changed, no new measurement is emitted (only the observation
timestamp advances) — an unchanged book does not get to look like new
evidence just because a heartbeat update arrived.

## Every metric this package produces

All published under `source=depthflow`, keyed `type:none` (one category wins
per observation; the reading is a single measurement, not per-side).

| Metric          | Meaning                                                                                                                                                                                                      |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `loaded_score`  | `\|weighted imbalance\|`, boosted or damped by trade-pressure agreement (see above). `0` unless this observation classified as loaded.                                                                       |
| `spoof_score`   | `\|weighted - level1\|` — how much the two imbalance resolutions disagree, in imbalance units (each imbalance itself is bounded in `[-1,1]`, so this can range up to `2`). `0` unless classified as spoofed. |
| `thin_score`    | `(baselineNotional - currentNotional) / baselineNotional` — how far below its own historical median the book's total notional currently sits. `0` unless classified as thinning.                             |
| `neutral_score` | `max(0, 1 - \|weighted\|)` — how far from any imbalance extreme the book currently sits. `0` unless classified as neutral.                                                                                   |
| `strength`      | `max` across the four scores above — whichever category won, its own reading.                                                                                                                                |
| `value`         | Same value as `strength`.                                                                                                                                                                                    |

### Normalization

`spoof_score` and, when the winning category is spoof, `strength`/`value`
are normalized against `2.0` (`maxBookImbalanceContrast`) — the domain-derived
maximum possible disagreement between two `[-1,1]`-bounded imbalance readings.
All other categories normalize `strength`/`value`/`loaded_score`/
`thin_score`/`neutral_score` against `1`, their natural bound. `Normalized`
is `nil` for any reading outside its category's valid domain — a defensive
check against the classifier's own invariants, not an expected runtime state.

### Readiness

A book only classifies once `WeightedHistory`/`Level1History` are non-empty
(`Bookflow.Measure` returns an empty, unready output otherwise) — there must
be at least one prior book observation to derive a threshold against.
`Maturity` (`Sample.maturity`) is `observations/(observations+1)`, asymptotic
toward 1 as more book observations accumulate for that symbol, mirroring the
same pattern used elsewhere in this codebase for confidence that grows
without a fixed calibration horizon.

## Files

| File        | Responsibility                                                                                    |
|-------------|---------------------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, book/trade dispatch and deduplication, measurement projection, normalization. |

The book state, weighted imbalance, toxicity penalty, and trade-pressure EMA
live in `nomagique/algorithm/book/flow`; the classification thresholds and
scoring live in `nomagique/equation/bookflow.go` — not here.

## What this package deliberately does not decide

A high `spoof_score` is not "this is definitely a spoof, veto the trade" — it
is "the touch and the deeper book disagree in direction, beyond this
symbol's own historical baseline, right now." Whether that disagreement is
manipulation, a large resting order simply not at the touch, or something
else is `logic/category`'s question — the `SpoofedPump` trap tape in
`tests/conditions` exercises exactly this reading against the evidence
graph's veto.

# `signal/liquidity` — is this symbol's touch unusually thin right now?

> Reported 24h volume tells you how much traded. It tells you nothing about
> how much you could execute right now without moving the price. Those are
> different quantities, and conflating them is the bug this package is
> written to avoid.

## What this package is

Liquidity is the **Scarcity perspective** — it measures how much notional is
actually **executable at the current best bid/ask**, right now, and compares
that against the same quantity for every other tracked symbol, to answer:
is this symbol's touch depth thin relative to its peers, right now? It
deliberately keeps that question separate from **reported turnover**
(exchange-reported 24h volume × price), which is retained purely as
context — the doc comment on `Signal` states this directly: reported-volume
notional "is retained as a separate turnover context and never mixed into
the book-depth score."

This package is entirely self-contained — no `nomagique` engine behind it
beyond `statistic.MedianOf`. No category, no trading gate — `logic/category`
decides what a reading means.

## Executable depth vs. reported notional: two different questions

```go
executableDepth = min(bidQty, askQty) × (bid+ask)/2
quoteNotional   = vwap (or last price) × 24h reported volume
```

`executableDepth` uses `min(bidQty, askQty)` — the smaller of the two touch
sides, since the amount you could actually trade *through* the touch on
either side is bounded by whichever side has less resting size, not the
average or the sum. This is "how much could I trade right now without
walking past the best price," which is a fundamentally different quantity
from "how much traded over the last day" (`quoteNotional`) — a symbol can
have enormous reported 24h volume and a razor-thin current touch (volume
concentrated in a few large prints, quiet otherwise), or the reverse. Both
are published, but never combined into one score.

## The cohort is time-aligned the same way as `sentiment`

`cohort()` uses the identical pattern to `signal/sentiment`: the median
observed inter-tick cadence across all symbols becomes the freshness window,
and a symbol whose most recent observation is older than that window behind
the cohort's latest is excluded from this tick's cohort — a symbol that's
gone quiet doesn't get to keep voting in the peer statistics using a stale
reading.

## Leave-one-out comparison: a symbol is never compared against itself

`leaveOneOutLiquidity` builds each symbol's peer comparison set by excluding
that symbol's own observation before computing the peer median
(`statistic.MedianOf`). This matters more here than in a simple cohort
average: without leave-one-out, a single very illiquid symbol would drag down
its own comparison baseline, making its own thinness look less extreme than
it actually is relative to everyone else. Comparing strictly against *other*
symbols avoids a symbol partially grading its own curve.

A peer comparison additionally requires at least 2 other symbols with a
positive reading (`peerReady`, `reportedReady`) and — separately — the whole
cohort needs at least `minimumLiquidityCohort` (3) symbols with a positive
value before a **cohort-level** median is trusted at all
(`liquidityCohortMedian`). These are two different readiness bars: the first
gates one symbol's own relative reading, the second gates the cross-cohort
context (`executable_touch_depth_median`,
`reported_volume_notional_median`) that lets a consumer compare *this
symbol's peer median* against the whole tracked universe's typical depth.

## Scarcity: a deficit scaled by how dispersed the peer group already is

```
deficit    = max(0, peerDepthMedian - thisSymbolDepth)
dispersion = median(|peerDepth - peerDepthMedian|)   // peer group's own spread
scarcity   = deficit / (deficit + dispersion)
```

This is the same "deficit relative to the group's own dispersion" pattern
used in `signal/sentiment`'s `leaderEvidence` and `signal/correlation`'s
energy scores: a symbol sitting modestly below a peer group whose depths are
themselves all over the place scores lower scarcity than one sitting the
same absolute distance below a *tightly clustered* peer group — because in
the tightly-clustered case, falling short by that amount is genuinely
unusual, whereas in the dispersed case it's within the peer group's normal
range of variation.

## Every metric this package produces

All published under `source=liquidity`, keyed `type:none`.

| Metric                            | Meaning                                                                                                                                                                                                                                           |
|-----------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `executable_touch_depth`          | `min(bidQty, askQty) × midpoint` — quote-currency notional executable at the current touch, this symbol. `Unit: quote_currency`.                                                                                                                  |
| `relative_touch_depth`            | This symbol's executable depth divided by its leave-one-out peer median — `1` is exactly typical, `<1` thinner than peers, `>1` deeper.                                                                                                           |
| `scarcity_score`                  | See above — how unusual this symbol's depth deficit is relative to peer dispersion, `0..1`. `0` unless this symbol's depth is actually below the peer median.                                                                                     |
| `executable_touch_depth_median`   | This symbol's own leave-one-out peer median depth. `Normalized`: that peer median expressed as a ratio against the **whole cohort's** median depth — is this symbol's peer group typically deep or thin relative to the broader tracked universe. |
| `reported_volume_notional`        | Exchange-reported turnover notional (24h volume × vwap/last), this symbol. Context only — never feeds `scarcity_score`.                                                                                                                           |
| `reported_volume_notional_median` | This symbol's leave-one-out peer median reported notional. `Normalized`: same cohort-relative ratio as the depth median above.                                                                                                                    |

### Readiness

`relative_touch_depth`, `scarcity_score`, and `executable_touch_depth_median`
all require: at least 2 peers with positive depth, a positive peer median,
cadence readiness, cohort-level depth readiness (≥3 cohort symbols with
positive depth), and a positive own reading. `reported_volume_notional`'s
normalized fields have the equivalent bar computed independently for the
notional series — a symbol can have ready depth evidence and not-yet-ready
notional evidence, or vice versa, since the two series can mature at
different rates.

## Files

| File        | Responsibility                                                                                                    |
|-------------|-------------------------------------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, per-symbol ingestion, cohort assembly, leave-one-out peer comparison, measurement projection. |

## What this package deliberately does not decide

A high `scarcity_score` is not "this symbol is illiquid, avoid it" — it is
"this symbol's currently executable touch depth is unusually thin relative
to its peer group's own dispersion, right now." Whether that's a genuine
liquidity risk or a transient quote gap is `logic/category`'s question,
weighed against everything else measured on the same tick.

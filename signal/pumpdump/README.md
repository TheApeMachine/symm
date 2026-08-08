# `signal/pumpdump` — is this the start of a pump, or the exhaustion of one?

> Wall-clock bars average away the exact thing a pump is made of: volume
> arriving faster than usual. A volume clock cannot average that away, because
> its bars are defined *by* volume arriving.

## What this package is

Pumpdump measures **ignition** — ticker lift, price precursor, and spread
compression, sampled on a **volume clock** instead of a wall-clock, plus
**exhaustion**: whether a move that was accelerating has started decelerating
while still rejecting further movement in the same direction. It answers "is
volume/price behavior at this instant unusual relative to this symbol's own
recent normal, in a way that precedes or accompanies rapid moves?"

This package is the market adapter around
[`nomagique/equation.Ignition`](../../../nomagique/equation/ignition.go). It
matches each trade to the reconstructed order book's best bid/ask at the
trade's own timestamp and feeds both into the volume-clock engine — no
category, no trading gate. That belongs to `logic/category`.

## The volume clock

Ordinary time-based bars ("one bar per second") let a burst of executed
volume get diluted across however many bars the burst happens to span in wall
time. A **volume bar** instead closes only once *enough volume has traded*,
regardless of how long that took — so a genuine burst of volume closes a bar
almost instantly, while a quiet stretch takes many wall-clock seconds to
accumulate the same bar.

Crucially, "enough volume" is not a fixed threshold. `advance`
([`nomagique/equation/window.go`](../../../nomagique/equation/window.go))
targets the **median positive volume advance already observed for this
symbol** (`statistic.MedianOf(window.deltas)`) — so a small-cap symbol that
normally trades in tiny clips and a large-cap symbol that trades in large
clips each get bars sized to their own typical trade size, not a global
constant. A bar only closes once accumulated volume reaches that target *and*
positive elapsed time has passed since the bar opened — a zero-duration bar
cannot produce a rate.

Between bar closes, an observation still updates the **live spread**
(`compose` overlays the current executable spread onto the last closed bar's
scores) — spread readiness does not wait for a volume bar to close, only the
directional/volume metrics do.

## Every score is a ratio against the symbol's own history

Nothing here is compared to a fixed threshold. Every family is `value /
median(retained history of that same quantity for this symbol)`
(`ignitionRatio`), so a reading of "2×" always means "twice this symbol's own
recent typical," whether that symbol usually moves in basis points or in
whole percent. This is the fix for the failure mode this signal previously
had (see project memory): without an adaptive per-symbol scale, an
illiquid symbol with naturally small typical values could see its scale
collapse to zero and every ratio derived from it degenerate.

## The two reciprocal directional families

Every closed bar produces **both** a `Buy` and a `Sell` `IgnitionSideOutput`
from the *same* observation — Buy scores the positive log-return case,
Sell scores the negative case, computed by feeding the same evidence with
signs flipped (`priceMove` vs. `-priceMove`). This is not two separate
detectors; it's one bar's evidence read through both directional lenses at
once, so a pump and a dump are structurally the same computation with the
sign reversed. The top-level (unsided) fields on `IgnitionOutput` carry
whichever of the two had the higher `Strength` this bar — "the stronger
directional story right now" — for callers that don't need the full
per-side breakdown.

## The four evidence families

Given this bar's RVOL ratio, its precursor ratio, and spread compression
(all already scaled against this symbol's own history), `ignitionFamilies`
(`nomagique/equation/evidence.go`) derives:

| Family        | Formula (informal)                                                                  | Reading                                                                                                                                                                                                                                                                                                                                                       |
|---------------|-------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Ignition`    | Geometric mean of RVOL and precursor ratio, when both are positive                  | Volume and directional price movement are elevated together — the core "something is happening" reading.                                                                                                                                                                                                                                                      |
| `Trend`       | Geometric mean of precursor ratio, a *squashed* RVOL ratio, and *quiet* compression | Directional movement continuing while volume is not (or no longer) spiking and spread has calmed — a move sustaining itself rather than one still being actively driven by a volume burst.                                                                                                                                                                    |
| `Compression` | `max(0, 1 - spread/median(spread))`                                                 | How much tighter the current spread is than this symbol's typical spread — the classic pre-move "the market is coiling" reading, independent of volume or price movement.                                                                                                                                                                                     |
| `Exhaustion`  | `max(0, priorRVOL - RVOL)/priorRVOL × squash(rejection, moveScale)`                 | Volume that was elevated is *now falling* (relative to the immediately prior bar, not the long-run baseline), combined with price rejecting further movement in this direction (`rejection` is scored against the *opposite* sign's move — see below). The signature of a move running out of the volume that was driving it while price stops confirming it. |

`Strength` is `max` across `Ignition, Compression, Trend, Exhaustion` — the
single strongest story this bar tells, whichever family produced it.

`buyRejection`/`sellRejection` are deliberately cross-wired:
`buyRejection = ignitionRatio(max(0, -priceMove), moveBaseline, ...)` — a
buy-side exhaustion reading needs the price to have moved in the *sell*
direction (rejecting further upside), not the buy direction. This is what the
doc comment on `ignitionExhaustion` means by "high-volume continuation cannot
masquerade as exhaustion": a bar with rising volume still pushing price the
same direction it was already going does not get to also claim exhaustion
credit.

## Squash and inverse: bounding without a hard cap

`ignitionSquash`/`ignitionInverse` (referenced by `ignitionFamilies`) map an
unbounded ratio into a comparable bounded evidence contribution, scaled by
`ignitionRatioScale`/`ignitionCompressionScale` — the *typical* value that
ratio takes for this symbol, so "elevated" is judged the same way "baseline"
was: empirically, per symbol, not by a shared constant across every market
this system watches.

## Every metric this package produces

All published under `source=pumpdump`. Unsided (`SideNone`) metrics carry the
stronger of Buy/Sell for that bar (the legacy top-level fields); `SideBuy`
and `SideSell` carry each directional family's own reading explicitly.

| Metric        | Sides           | Meaning                                                                                                                                                                                      |
|---------------|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `rvol`        | none            | Relative volume: this bar's volume rate divided by this symbol's median historical bar rate. Not sided — RVOL is a property of the bar, not a direction.                                     |
| `precursor`   | none, buy, sell | Absolute (unsided) / positive-only (buy) / negative-only (sell) log-return this bar, each scaled against this symbol's median absolute historical bar move.                                  |
| `spread`      | none            | Current executable ask-minus-bid, in quote currency (`Unit: quote_currency`). `Normalized` is spread as a fraction of the book midpoint at the trade. Overlaid live even between bar closes. |
| `compression` | none, buy, sell | How much tighter the current spread is than this symbol's typical spread, `0..1`+ scaled. Same value both sides carry independently through `ignitionFamilies`.                              |
| `ignition`    | none, buy, sell | The core RVOL×precursor evidence family for that direction.                                                                                                                                  |
| `trend`       | none, buy, sell | Directional movement continuing under calming volume/spread, for that direction.                                                                                                             |
| `exhaustion`  | none, buy, sell | Falling volume plus opposite-direction price rejection, for that direction — see cross-wiring above.                                                                                         |
| `strength`    | none, buy, sell | `max` across that direction's four families.                                                                                                                                                 |

### Readiness

`Normalized` for every metric is `nil` until `ready` is true —
`window.classified`, which requires both the volume-rate baseline *and* the
spread baseline to have matured (`rateReady && spreadReady`, both from
`statistic.MedianOf` on retained history). Before that, `Raw` fields are
still populated (often zero) so the reading stays visible as "not yet
calibrated" rather than absent. `Maturity` is `bars/(bars+1)` — asymptotic
toward 1 as more volume bars close, with no fixed horizon.

`spread`'s `Normalized` has its own gate (`normalizedSpread`, in
[`signal.go`](signal.go)): it requires only a positive spread and midpoint,
independent of `ready`, since the live spread is meaningful even before a
volume bar has closed.

## History retention

Each symbol's `deltas`, `rates`, `returns`, `precursors`, and `spreads` are
bounded ring buffers (`ignitionWindow.append`), capped at
`signals.pumpdump.baselineCapacity` from config — the same explicit retention
bound the production market feed uses, so this package's empirical baselines
and the feed's own retained history stay consistent in size.

## Files

| File        | Responsibility                                                                       |
|-------------|--------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, trade/book matching, per-symbol fan-out, measurement projection. |

The volume-clock engine — bar closing, evidence families, exhaustion,
adaptive scaling — lives in `nomagique/equation/ignition.go`,
`window.go`, and `evidence.go`, not here.

## What this package deliberately does not decide

A high `ignition` reading is not "a pump is starting, buy it" — it is "volume
and directional price movement are both elevated relative to this symbol's
own recent history, right now." Whether that's the start of a real move, a
wash-trade artifact, or a spoofed pump trap is `logic/category`'s question —
see the `SpoofedPump` trap tape in `tests/conditions`, which exists precisely
to prove the evidence graph vetoes a reading like this when depth doesn't
corroborate it.

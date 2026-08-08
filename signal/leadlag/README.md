# `signal/leadlag` — who is moving first, and is everyone else keeping up?

> A follower correlated with the leader is not interesting. A follower that
> *used to be* correlated and has gone quiet is the tell.

## What this package is

Lead-lag is the **Anchor perspective** — it picks the cross-section's current
price leader and measures every other symbol's temporal relationship to it:
does this follower move in lockstep with the leader (synchronized), does it
move a measurable number of bars *after* the leader (inefficient — a lag a
model could exploit), or has it stopped responding to the leader's moves at
all (decoupled, or stalled if the leader itself hasn't moved yet)?

This package is the market adapter around
[`nomagique/algorithm`](https://github.com/TheApeMachine/nomagique/tree/b07bb443d986395a2ab11f7e9969b3d2fe55299a/algorithm)'s cross-lag and
Hayashi-Yoshida correlation primitives. It owns which symbol is "the anchor"
right now, retains each symbol's aligned price path, and turns lag/sync
evidence into a bounded, category-agnostic score — `logic/category` decides
what a reading means for a given follower.

## The anchor is derived, not configured

There is no config-file "major" symbol. `Section.CausalAnchor()` picks the
anchor **fresh, every cycle**, from whichever symbol just produced the largest
absolute log-return move in the current cross-section — but only if that move
exceeds the **median** move magnitude across all symbols this tick. If nothing
stands out from the pack, there is no anchor and the cohort emits provisional
(all-zero) evidence rather than measuring lag against a leader that isn't
actually leading anything.

This is deliberately causal: `CausalAnchor` only ever looks at the return
already recorded on the *previous* price observation versus the one before
it — never the frame currently being ingested. A symbol cannot become its own
anchor by virtue of the very move being scored this tick; leadership has to
have already happened.

When leadership rotates to a new symbol, the anchor's buffered move-baseline
history is reset (`SetAnchor`) — an old leader's move statistics don't carry
over and bias what counts as a "significant move" for the new one.

## Two kinds of correlation, and why both are needed

For each follower, `Section.Features` computes two distinct temporal
relationships against the anchor's price path:

1. **Contemporaneous correlation** (`algorithm.HayashiPairCorrelation`, zero
   lag) — the Hayashi-Yoshida estimator, which correlates asynchronously
   time-stamped price paths without requiring them to share a common clock.
   This is "how correlated are these two series *right now*."
2. **Lag correlation** (`algorithm.CrossLagScore`) — searches shifted copies
   of the anchor's path against the follower's, up to
   `resolvedMaxLagBars(sampleCount)` bars, and keeps the shift that produces
   the strongest Hayashi correlation. This is "if the follower is just
   delayed, how many bars behind, and how strong is the fit once that delay is
   accounted for."

A lag correlation is only trusted if it clears a **significance threshold**
derived from how many independent shifts were searched
(`lagSearchThreshold` — a Bonferroni-style `√(2·ln(searches)/effectiveSupport)`
bound): searching more lag candidates makes it easier to find *some* shift
that correlates well by chance, so the bar for "this lag is real" rises with
the number of shifts tried, not just the correlation itself.

`selectCorrelations` then picks whichever of contemporaneous or (significant)
lag correlation is stronger as the reading's `signedCorrelation` — a follower
is scored by its best-supported relationship to the anchor, not forced into
one or the other.

## Four evidence weights, not one score

`weightEvidence` combines the correlation selection with two gates — whether
the anchor is actually moving right now (`MoveReady`/`MoveMoved`, from
`Section.recordAnchorMove` and `algorithm.MoveBaseline`, which tracks the
anchor's own historical move sizes to decide what counts as a "significant"
move for it) and how much of the observed correlation is lag vs.
contemporaneous — into four independent evidence weights, each in `0..1`:

| Weight                 | Fires when                                                                                                                             | Reading                                                                                         |
|------------------------|----------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| `inefficient`          | Anchor moving, meaningful lag correlation, low stall margin                                                                            | The follower is trailing the anchor by a measurable number of bars — a real, exploitable delay. |
| `sync` (`Sync` metric) | Anchor moving, strong contemporaneous correlation, little lag fraction                                                                 | The follower is moving with the anchor essentially immediately — no delay to trade against.     |
| `decoupled`            | Low correlation of either kind, low stall margin                                                                                       | The follower is not tracking the anchor's moves at all right now.                               |
| `stall`                | Uncorrelated, no lag, but the anchor itself hasn't produced a significant move yet (`stallDamp` zeroes this once the anchor does move) | Nothing has happened yet to test the relationship — this is "waiting," not "decoupled."         |

All four are additionally scaled by `sampleSupport` (how much of the resolved
short window's depth is actually filled — `sampleSupportFraction`) and by
whether the anchor's move baseline has enough history to be trusted
(`anchorActive`). `strength` is the max across all four, so a consumer asking
"how strong was this follower's reading, regardless of which relationship won"
doesn't need to inspect all four separately.

## The adaptive window

`windowsFromCount` (`windows.go`) derives short/long window sizes and a
return-lag purely from how many price samples exist so far — `shortWindow =
⌈√n⌉`, `longWindow ≈ 2×shortWindow`, `returnLag = ⌈√longWindow⌉` — the same
resolution `nomagique/statistic.ResolveWindows` would produce on a
zero-filled series of that length, computed directly to avoid allocating that
series every call. Retention (`priceRetentionCount`), max lag search depth
(`resolvedMaxLagBars`), and the minimum-observations floor for detecting an
anchor move (`resolvedShortWindow`) are all derived from this, so every
symbol's window scales to how much history it personally has rather than a
fixed bar count shared across illiquid and liquid markets alike.

Price observations are also rate-limited to the series' own median sample
spacing (`ObservePrice`, `seriesSampleSpacing`) — a burst of ticks faster than
the symbol's typical update cadence does not get treated as new independent
observations.

## Every metric this package produces

All published under `source=leadlag`, keyed `type:none`. `Peer` on the
measurement carries the anchor's symbol when this row is a follower (empty
when this symbol *is* the anchor, or there is no anchor), so a directed
Leads/Lags edge can be drawn between the two in the evidence graph.

| Metric                       | Meaning                                                                                                                                                                                                                                                |
|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `correlation`                | `\|signedCorrelation\|` — how strongly related this follower is to the anchor right now, unsigned, `0..1`.                                                                                                                                             |
| `signed_correlation`         | The selected (contemporaneous-or-lag, whichever is stronger) correlation with sign — direction of co-movement, `-1..1`.                                                                                                                                |
| `signed_contemp_correlation` | The zero-lag Hayashi-Yoshida correlation alone, signed, `-1..1`.                                                                                                                                                                                       |
| `signed_lag_correlation`     | The best-shift lag correlation alone, signed, `-1..1`. Zero unless a significant lag was found.                                                                                                                                                        |
| `lag_fraction`               | `\|lagBars\| / maxLagBars` — how much of the searchable lag depth the best-fit shift used, `0..1`. Near-zero means "essentially no lag"; near-one means "lagging by close to the maximum bars this window can even test."                              |
| `sample_support`             | How much of the resolved short window is actually filled with observations yet, `0..1` — a maturity gauge independent of the correlation readings themselves.                                                                                          |
| `inefficient`                | Evidence weight for "this follower lags the anchor by a real, significant delay."                                                                                                                                                                      |
| `sync`                       | Evidence weight for "this follower moves with the anchor essentially immediately."                                                                                                                                                                     |
| `decoupled`                  | Evidence weight for "this follower is not tracking the anchor at all right now."                                                                                                                                                                       |
| `stall`                      | Evidence weight for "nothing has been tested yet — the anchor hasn't moved enough to judge this relationship."                                                                                                                                         |
| `strength`                   | `max` of the four evidence weights — the winning relationship's own score.                                                                                                                                                                             |
| `signed_lag_direction`       | `+1` if the anchor leads this follower (a significant lag was found), `0` otherwise. Always emitted (even on idle/self-anchor ticks) so the metric's identity — and downstream code that reads it — stays stable whether or not a lag fired this tick. |

### Provisional readings

While no symbol currently qualifies as anchor, every symbol gets a
`provisional` measurement: every metric present, every `Raw` zero. This keeps
a quote tick from going silent on this signal entirely during leaderless
periods, while making the all-zero state unambiguous — it is not "no
relationship found," it is "there is currently no anchor to relate to."

## Files

| File              | Responsibility                                                                               |
|-------------------|----------------------------------------------------------------------------------------------|
| `signal.go`       | `Signal` lifecycle, dispatch into `measureFrame`.                                            |
| `measurements.go` | Frame ingestion, correlation selection, evidence weighting, measurement construction.        |
| `section.go`      | Anchor selection, per-symbol price/return retention, lag/contemporaneous feature extraction. |
| `windows.go`      | Sample-count-derived window sizing shared by `section.go` and `measurements.go`.             |

The correlation and lag-search primitives (`HayashiPairCorrelation`,
`CrossLagScore`, `MoveBaseline`) live in `nomagique/algorithm`, not here.

## What this package deliberately does not decide

A high `inefficient` score is not "trade this lag" — it is "this follower has
historically lagged the anchor by a measurable, significant amount, right
now." Whether that lag is stable enough, large enough, and survives
transaction costs is `logic/category`'s and the strategy layer's question, not
this signal's.

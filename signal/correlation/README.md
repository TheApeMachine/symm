# `signal/correlation` — is this symbol moving with the crowd, or alone?

> Correlation alone says "these two move together." What matters for trading
> is *whose* move is bigger when they do — that's the difference between
> herding and having an edge.

## What this package is

Correlation is the **cohort perspective** — for every symbol, it asks how that
symbol's price moves relate to the rest of the tracked universe's moves, right
now: is it moving in the same direction as its correlated peers (herding), is
it moving *more* than they are while still correlated (a real, possibly
tradeable divergence — alpha), is it moving *less* than expected given how
correlated it is (noise), or is it moving *against* the direction its peers
are moving (stress)?

This is a pairwise, cross-sectional signal — every symbol is scored against
*every other tracked symbol at once*, weighted by how much overlapping
evidence each pair actually has. It uses
[`nomagique/correlation`](../../../nomagique/correlation)'s sample type but
otherwise implements its own streaming Hayashi-Yoshida estimator and cohort
aggregation directly in this package (`hayashi.go`, `section.go`) — there is
no external cohort-scoring engine being wrapped here, unlike most other
signals.

## Why Hayashi-Yoshida, not ordinary correlation

Symbols do not tick at the same instants, and forcing them onto a shared
clock (resampling to fixed bars) either throws away information or invents
prices that were never observed. The Hayashi-Yoshida estimator sidesteps
this: it sums the product of two symbols' returns over every pair of
overlapping observation *intervals* — not synchronized points — so
asynchronously sampled price paths can still be correlated exactly.

`hayashiMoments` (`hayashi.go`) computes covariance and both symbols'
variances using **only the intervals that participate in at least one
overlap** — an interval from either symbol's price path that never overlaps
anything on the other side does not get to inflate that symbol's own
variance and dilute the correlation. This matters because a symbol that ticks
much faster than its peer would otherwise have most of its variance come from
returns the peer's price path can't even see.

## Correlation is maintained incrementally, not by rescanning

`symbolState` retains each symbol's full sample path, precomputed log-prices,
and its own Hayashi variance (`exactVariance`, `pair.go`) so a new observation
does not require rebuilding statistics from scratch. `exactVariance` recomputes
the variance sum exactly from retained log-prices on every append rather than
maintaining a running `± ret²` accumulator — the code deliberately avoids
streaming floating-point accumulation error creeping into a denominator that
every pairwise correlation this symbol participates in depends on.

The retention window itself is adaptive: `trim` calls
`statistic.ResolveWindows` on the symbol's own return series to derive
`longWindow`, and only re-resolves it once the path has grown past the
previous window plus one sample — so window sizing tracks a symbol's own
observed volatility/sample-rate character without being recomputed on every
single tick.

## The four cohort scores

`scores` (`section.go`) computes, for one symbol against the whole tracked
universe:

```
weightedSigned    = Σ over peers  signedCorrelation(symbol, peer) × support(pair)
weightedAbsolute  = Σ over peers  |correlation(symbol, peer)|      × support(pair)
weightedPeerEnergy = Σ over peers  peer.energy                      × support(pair)
                     ────────────────────────────────────────────────────
                                    Σ support(pair)
```

Every peer's contribution is weighted by **support** — the number of
overlapping Hayashi intervals that pair actually shares (from
`supportedCorrelation`) — so a peer with a thin, barely-overlapping price
history cannot sway the cohort average as much as one with a well-supported
comparison. `energy` (per symbol, `refreshEnergy`) is the **median absolute
value of the symbol's own time-normalized returns** — `Δlog(price)/√Δt` —
i.e. this symbol's own typical volatility, scale-normalized so a fast-ticking
symbol and a slow-ticking one are compared fairly (`fillReturns`).

From the weighted cohort correlation (`signed`, `correlation`) and this
symbol's energy relative to its peers' (`relativeEnergy = energy/peerEnergy`):

```
excessEnergy  = max(0, relativeEnergy - 1)   // how far above peer-typical this symbol's move is
energyDeficit = max(0, 1 - relativeEnergy)   // how far below peer-typical
excessMass    = excessEnergy / (1 + excessEnergy)

herdScore  = max(0, signed) / (1 + excessEnergy)
alphaScore = excessMass / (1 + max(0, signed))
noiseScore = max(0, 1-correlation) / (1 + excessEnergy + energyDeficit)
stressScore = max(0, -signed)
```

Each formula is a distinct question about the same underlying numbers:

| Score         | Question it answers                                                                                                                                                                                                                                                                                                                                 |
|---------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `herdScore`   | Is this symbol correlated *and* not moving unusually large relative to peers? High when signed correlation is strong and this symbol's move is proportionate — the textbook "moving with the crowd, nothing special" reading. Damped by `excessEnergy`, so a symbol that's correlated but moving abnormally large no longer counts as pure herding. |
| `alphaScore`  | Is this symbol moving unusually large relative to peers *without* that move being explained by high positive correlation? High `excessMass` with low `signed` correlation is the signature of a move that peers are not simply dragging along — the candidate "this symbol found something on its own" reading.                                     |
| `noiseScore`  | Is this symbol both uncorrelated with peers *and* moving an unremarkable amount? Damped by both `excessEnergy` and `energyDeficit`, so an unusual move in either direction pulls this down — noise specifically means "nothing distinguishing is happening here."                                                                                   |
| `stressScore` | Is this symbol moving *against* its cohort's direction? `max(0, -signed)` — nonzero exactly when the weighted correlation is negative, i.e. this symbol and its peers are, on net, moving opposite ways.                                                                                                                                            |

`strength` (also published as `peakScore` — both keys currently carry the
same `max` value) is `max` across all four, so a consumer wanting "how
distinctive was this symbol's cohort relationship this tick" doesn't need to
know which of the four categories produced it.

A zero-strength reading is still emitted deliberately (see the comment at
`section.go:362`) — a quiet, fully-locked cohort is a real state the rest of
the system needs to see, not something to suppress as "no signal."

## Every metric this package produces

All published under `source=correlation`, keyed `type:none`.

| Metric            | Meaning                                                                                                                                                                                            |
|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `correlation`     | Support-weighted mean of `\|signed correlation\|` across all peers — how correlated this symbol is with its cohort overall, unsigned, `0..1`.                                                      |
| `signed`          | Support-weighted mean of signed correlation across all peers — net direction of cohort co-movement, `-1..1`.                                                                                       |
| `relative_energy` | This symbol's own median time-normalized return magnitude divided by its peers' support-weighted average — `1` means typical, `>1` means moving more than peers right now, `<1` means moving less. |
| `herd_score`      | See table above. `0..1`, high = correlated and proportionate.                                                                                                                                      |
| `alpha_score`     | See table above. `0..1`, high = large move not explained by correlation.                                                                                                                           |
| `noise_score`     | See table above. `0..1`, high = uncorrelated and unremarkable move.                                                                                                                                |
| `stress_score`    | See table above. `0..1`, high = moving against the cohort's net direction.                                                                                                                         |
| `peak_score`      | Same value as `strength` (`max` of the four scores).                                                                                                                                               |
| `strength`        | `max(herd_score, alpha_score, noise_score, stress_score)`.                                                                                                                                         |

### Readiness

A symbol only produces scores once it has at least one peer with a positive,
well-supported (`support ≥ 2` on both sides — see `hayashiMoments`) pairwise
correlation and positive energy on both sides (`scores` returns `false`
otherwise, and that symbol is silently omitted from the tick's measurements —
not published with nils).

## Files

| File         | Responsibility                                                                                |
|--------------|-----------------------------------------------------------------------------------------------|
| `signal.go`  | `Signal` lifecycle, dispatch, measurement projection and normalization.                       |
| `section.go` | Per-symbol streaming state, incremental variance/energy maintenance, cohort score derivation. |
| `hayashi.go` | The Hayashi-Yoshida covariance/variance estimator used for every pairwise correlation.        |
| `pair.go`    | Exact-variance recomputation helper shared by `section.go`.                                   |

## What this package deliberately does not decide

A high `alpha_score` is not "this symbol has an edge, trade it" — it is "this
symbol is moving more than its cohort without that move being explained by
correlation, right now." Whether that divergence is informative, a data
artifact, or about to mean-revert is `logic/category`'s question, weighed
against everything else measured on the same tick.

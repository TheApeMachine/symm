# `signal/sentiment` — is the whole market moving, or just one symbol?

> A big move in one symbol and the same-sized move confirmed by the rest of
> the market are different events, even if the number looks identical.

## What this package is

Sentiment is the **cross-sectional breadth and leadership perspective** — it
does not measure any one symbol in isolation. Every tick, it looks at *every*
tracked symbol's return at once and asks: is the cohort broadly advancing or
declining (breadth), is there a symbol moving unusually large relative to the
rest (a leader), and if so, is the rest of the cohort confirming that
leader's direction or is the leader diverging from an otherwise indifferent
or opposing market?

This package is a pure `nomagique` composition built on `nomagique.Number[string](temporal.Path)`
and `nomagique/algo.CohortSentiment`. No category, no trading gate — `logic/category` decides what
a reading means.

## The cohort is time-aligned by median cadence, not by a fixed clock

`cohort()` computes the **median observed update cadence** across all
symbols with a ready return (`statistic.MedianOf` on each symbol's own
inter-tick gap) and uses that as a **freshness window**: a symbol is only
included in this tick's cohort if its most recent observation is no older
than that median cadence behind the cohort's latest observation
(`latest.Sub(observation.at) > freshness` excludes it). This keeps a symbol
that has gone quiet (stopped ticking) from continuing to vote in the cohort
statistics using a stale return from many ticks ago, without requiring a
fixed staleness constant that would be wrong for both fast- and slow-ticking
symbols simultaneously.

## Breadth: how one-sided is the whole cohort, right now

```
breadth = (advances - declines) / len(peers)
```

Simple advance/decline breadth, in `-1..1`. This is a pure counting statistic
— it does not care *how much* each symbol moved, only which direction the
majority of symbols moved. A cohort where every symbol ticked up by a tiny
amount and a cohort where every symbol ticked up by a huge amount produce the
same breadth reading; magnitude is a separate question, answered by surge and
slump.

## Surge and slump: is the *typical* move directional and does the cohort agree

```
medianChange, medianMagnitude = median(returns), median(|returns|)
agreement = max(advances, declines) / len(peers)

surge = max(0, medianChange) × agreement / medianMagnitude
slump = max(0, -medianChange) × agreement / medianMagnitude
```

These require **both** conditions to be strong to score high: the cohort's
*median* return has to be directional (not the mean, which one large mover
could skew — the median is what most symbols are actually doing), and
`agreement` scales the reading down when the cohort is split roughly evenly
between advancers and decliners even if the median itself leans one way.
Dividing by `medianMagnitude` expresses the median directional move as a
multiple of the cohort's own typical move size this tick — a self-relative
scale, not an absolute return threshold.

## Leadership: is one symbol standing out, and is the rest of the cohort backing it up

The **leader** is simply whichever symbol has the largest absolute return
this tick (`leaderMagnitude`). Everything past that identification is about
whether that leadership is *evidenced*:

```
relativeLead  = leaderMagnitude / Σ all magnitudes
```

`relativeLead` is how much of the cohort's total absolute movement this tick
belongs to the leader alone — a leader responsible for most of the cohort's
total movement is a much stronger reading than one that's merely the largest
of many similarly-sized moves.

```
peerMedian, peerDispersion = median(peer magnitudes), median(|peer magnitude - peerMedian|)
excess = leaderMagnitude - peerMedian
leaderEvidence = excess / (excess + peerDispersion)
```

`leaderEvidence` asks a sharper question than `relativeLead`: is the leader's
move large not just in *sum* but relative to how *dispersed* the rest of the
cohort's magnitudes already are? A leader that's only modestly ahead of a
tightly-clustered peer group scores lower than one that's far ahead of peers
whose own magnitudes are all over the place — the denominator uses the peer
group's own dispersion as the yardstick for what "far ahead" means for this
particular cohort, this tick. `leaderEvidence` is only computed at all when
`leaderMagnitude > peerMedian` — a "leader" that isn't even above the peer
group's typical magnitude gets no evidence credit regardless of the other
numbers.

## Divergence: the leader is real, but the cohort disagrees with its direction

```
nonconfirming = count of peers where sign(peerChange) != sign(leaderChange)
divergence    = leaderEvidence × nonconfirming / len(peer magnitudes)
```

This is the most information-dense reading this package produces: it only
scores high when the leader's move is well-evidenced (`leaderEvidence`
already high) **and** a large share of the rest of the cohort is moving the
*opposite* direction. A leader everyone else is confirming scores low
divergence regardless of how strong its evidence is — divergence
specifically flags "one symbol is running hard while its peers are not
following, or are actively going the other way."

## Every metric this package produces

All published under `source=sentiment`, keyed `type:none`. Every symbol in
the cohort gets the **same** breadth/surge/slump/divergence readings this
tick (they are cohort-level statistics, not per-symbol) — only `change` (this
symbol's own return) and the leader fields (nonzero only for the symbol that
actually is the leader) vary per symbol.

| Metric            | Meaning                                                                                                                            |
|-------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `change`          | This symbol's own log return since its previous observation.                                                                       |
| `breadth`         | Cohort-level advance/decline balance, `-1..1`. Same value for every symbol in the cohort this tick.                                |
| `leader_strength` | The leader's own magnitude (`0` for every non-leader symbol).                                                                      |
| `leader_evidence` | See above — how much the leader stands out relative to peer dispersion. `0` for non-leader symbols.                                |
| `relative_lead`   | The leader's share of the cohort's total absolute movement. `0` for non-leader symbols.                                            |
| `surge_score`     | Cohort-level "typical move is up and the cohort agrees," `0..1`. Same for every symbol. Normalized in `[0,1]`.                    |
| `divergent_score` | See above — leader evidenced but cohort not confirming. Only nonzero for the leader symbol. Normalized in `[0,1]`.                 |
| `slump_score`     | Cohort-level "typical move is down and the cohort agrees," `0..1`. Same for every symbol. Normalized in `[0,1]`.                   |
| `strength`        | `max(surge_score, divergent_score, slump_score)` — whichever cohort-level story is currently strongest. Normalized in `[0,1]`.      |

### Readiness

A symbol only enters the cohort once it has at least two observations (so a
return can be computed — `observation.ready`) and its latest observation
falls within the cohort's freshness window. Without cohort-wide movement to
compare against, there is no scale to evaluate cohort baseline dynamics.

## Files

| File        | Responsibility                                                                                                           |
|-------------|--------------------------------------------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, per-symbol return ingestion, cohort assembly, breadth/leadership statistics, measurement projection. |

## What this package deliberately does not decide

A high `divergent_score` is not "the leader is wrong, fade it" — it is "one
symbol is moving unusually large right now and the rest of the tracked
cohort is not confirming that direction." Whether that's an early signal, a
symbol-specific event unrelated to the broader market, or noise is
`logic/category`'s question, weighed against everything else measured on the
same tick.

# `logic/category` — what is the market *doing*?

> Aggressive drive with thinning depth is a breakout.
> Aggressive drive into loaded depth is absorption.
> Same drive. Opposite trades.

## What this package is

Category is the first stage in the logic chain, and it answers the most basic
question in the system: **what kind of market is this right now?**

A category is a *hypothesis* — `aggressive_drive`, `hidden_absorption`,
`coiled_compression`, `spoof_trap`, `book_thinning`, `laminar`, `turbulent` — and
every metric that carries affinity for it is typed evidence for or against.
The stage does not classify. It runs an evidence tally and reports what survives,
along with what argued the other way and what never showed up.

This is deliberately the shallowest stage mathematically and one of the most
important structurally, because everything downstream inherits its framing.

## The core insight: categories are cross-signal

**No single signal names a category.** That is the whole design.

The same reading means different things depending on what the rest of the market
is doing. Order-flow aggression alone is meaningless — it is a breakout *or* an
absorption depending entirely on what the depth is doing underneath it. A system
that lets one signal declare a regime will confidently trade the wrong side of
the same tape.

So `Mapper` holds a weight matrix: metric → category → weight, where sign is
direction of evidence and magnitude is how much that one observable is worth
relative to the others voting on the same hypothesis.

## Every metric speaks twice

```
metric ──┬── level  →  what is true now
         └── slope  →  where it is heading
```

`Mapper` holds two weight sets: `metrics` (level) and `trending` (direction).
They are scored independently because a metric can carry one without the other.

This is not a refinement, it is the point. Compression that is *still tightening*
is a coil. Compression sitting flat is a quiet market. Same level, different
market.

> A coiled compression is the setup that precedes a vertical ignition: spread
> tightening while arrivals build, with the depth that would explain it away as a
> spoof absent. Catching it is worth more than catching the ignition, because by
> the time price is vertical the fills are already poor.

Trend-derived evidence is tagged with a `↑` suffix in the provenance, so a
verdict can show that *tightening* carried it rather than the level.

## The tally

For each reading, for each category it speaks to:

```
contribution = |reading| × |weight|

sign(reading) == sign(weight)  →  support
else                           →  opposition
```

A rising metric supports the categories it is positively weighted for; a falling
one supports their opposites. One weight entry therefore covers both directions,
which is why the matrix stays readable.

**Each observable contributes exactly once**, at the strength it reads now.
Repeating a metric across every row in the tick would let a chatty signal outvote
the entire rest of the market — a pure artifact of publication rate.

## Claiming a category

Two gates, both non-negotiable:

```go
if len(found.distinct) < minimumEvidence { continue }   // ≥ 2 distinct observables
if confidence <= 0.5 { continue }                       // support must outweigh opposition
```

> One observable agreeing with itself is not a market regime, it is a reading.

`distinct` counts separate *observables*, not rows — ten readings of one metric is
one piece of evidence.

## What a verdict carries

| Field        | Meaning                                               |
|--------------|-------------------------------------------------------|
| `Confidence` | `support / (support + opposition)`                    |
| `Strength`   | `support − opposition` — absolute weight, not balance |
| `Maturity`   | Mean maturity of the contributing readings            |
| `Surprisal`  | `−log(1 − confidence)`                                |
| `Supporting` | Which observables argued for                          |
| `Opposing`   | Which argued against                                  |
| `Missing`    | Metrics with an opinion here that **did not appear**  |

Three of these deserve attention:

**Confidence and Strength are different questions.** Confidence is how one-sided
the evidence was. Strength is how much evidence there was. A unanimous verdict
from two weak readings and a contested verdict from twenty strong ones are not
interchangeable, and collapsing them into one number loses the distinction.

**Surprisal is the informative quantity.** It measures the information carried by
the *contested* share of the evidence. A category the observables agree on
unanimously tells you nothing you did not already see. One held against real
contradiction is the interesting case — and the tail of `−log(1−c)` is what makes
it stand out.

**`Missing` is honesty about absence.** A verdict states what it did not get to
see rather than silently assuming it. A breakout claimed without depth having
spoken is a materially weaker claim than one where depth spoke and agreed, and
the verdict has to be able to say which it was.

## Where categories go

Categories become **hypothesis nodes** in the evidence graph (`logic/graph`),
which relates the other stages' conclusions to them with `supports` /
`contradicts` edges. That graph then gates entries — the `evidence_opposition`
cause can veto a trade outright. Trap tapes (`SpoofedPump`, `Vacuum`, `Coil`) in
`tests/conditions` exist to prove that veto fires.

Category types live in [`types/category.go`](../../types/category.go).

## Files

| File        | Responsibility                                              |
|-------------|-------------------------------------------------------------|
| `solver.go` | Collection, tallying, verdicts, missing-evidence reporting. |
| `mapper.go` | The metric → category weight matrix, level and trending.    |

## Extending the matrix

When adding a metric's affinity, the question to answer is not "does this metric
relate to this category" — almost everything relates to everything. It is:

> If this metric were the *only* thing I knew, how much would it move my belief
> about this hypothesis, relative to the other observables that vote on it?

And separately: does its **direction** carry information the level does not? If
so it belongs in `trending` too, and that is where the anticipatory categories
come from.

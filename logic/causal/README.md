# `logic/causal` — climbing Pearl's ladder

> Correlation is rung one. A trading system that stops there is betting on
> coincidence.

## What this package is

This stage evaluates **Judea Pearl's Causal Ladder** over live market
observations. The ladder is Pearl's hierarchy of what a model can answer, and the
rungs are genuinely different kinds of question:

| Rung                  | Question                    | Notation           | In this system                                                         |
|-----------------------|-----------------------------|--------------------|------------------------------------------------------------------------|
| **1. Association**    | What is observed together?  | `P(y \| x)`        | Does forecast co-move with realized return?                            |
| **2. Intervention**   | What if I *act*?            | `P(y \| do(x))`    | If the forecast were forced to this level, what return follows?        |
| **3. Counterfactual** | What *would have* happened? | `P(y_x \| x', y')` | Given what actually happened, what if the forecast had been different? |

The distinction is not academic. Association will happily tell you that ice cream
sales predict drownings. Rung two asks what happens when you *intervene* on ice
cream sales, and the answer is nothing — summer was the confounder. A trading
signal that only clears rung one has exactly this failure mode: it tracks
something that tracks the market, and it dies the moment the shared cause moves.

The ladder math lives in [`nomagique/causal`](../../../nomagique/causal), wrapped
by `algorithm.Pearl`. This package decides what the variables *are*.

## The causal model

Everything hinges on the four-column row. This is the system's actual causal
hypothesis about itself, written down:

```
row = [ Energy , Surprise , Prediction , RealizedReturn ]
        └─ control ─┘        └treatment┘   └── target ──┘
```

| Column                  | Role          | Source                               |
|-------------------------|---------------|--------------------------------------|
| 0 — **Energy**          | Control       | Resonance system energy              |
| 1 — **Surprise**        | Control       | Resonance prediction error / anomaly |
| 2 — **Prediction**      | **Treatment** | Resonance forecast, step 0           |
| 3 — **Realized return** | **Target**    | Sentiment `change` metric, latest    |

Read as a claim: *does the predictive-coding forecast actually cause realized
return, once we adjust for how energetic and how surprised the field was?*

Energy and Surprise are controls because they are the obvious confounders. A
high-energy, high-surprise regime moves price *and* moves the forecast. Without
adjusting for them, the forecast would look causal when it is merely
co-symptomatic — the ice cream problem, in market clothing.

`NonlinearCounterfactual` is enabled: the counterfactual is not assumed linear in
the treatment.

## Rung 1 is where most signals die

The output carries both raw and scored quantities at every rung — `Association`,
`Intervention`, `DoExpectation`, `Uplift`, `Counterfactual`, plus `Noise`,
`Contagion`, and `Condition`. `Uplift` is the one that matters most for trading:
the expected difference in target under intervention versus not.

Contagion is fed by Surprise and Condition by Energy, so the ladder knows which
regime each row was observed in rather than pooling all regimes into one average.

## Readiness has two gates, and they are separate

```go
searchReady := ready && len(rows) >= MinimumSearchRows   // 12
```

- **`ready`** — the Pearl model's own judgement that it has enough to estimate.
- **`MinimumSearchRows`** — the live MCTS contract downstream. Twelve aligned
  observations before a *decision* may rest on this.

A warm predictive-coding reading is still a causal *result*: the symbol simply is
not ready for search. The solver publishes `{"ready": false}` explicitly rather
than staying silent, so the planner can complete an honest `ActionNothing`
decision while resonance keeps learning. Silence downstream is indistinguishable
from a stalled stage; an explicit not-ready is not.

## History depth is derived from the model, not chosen

```go
capacity := 1 + rowWidth + rowWidth*(rowWidth+1)/2
```

This is exactly the number of first- and second-moment parameters implied by the
row width — the intercept, the means, and the covariance upper triangle. For the
four-column row that is 15 rows retained.

The point is that the window is *the model's own data requirement*, not an
independent tuning knob someone picked. Widen the row and the window widens with
it, automatically and for a reason.

## Ordering

Causal runs **after** resonance in the analyzer chain and consumes
`thesis.Resonance`. This is a hard dependency, not a convenience: three of the
four columns are resonance outputs. The forecast must validate (`Forecast.Validate()`)
and step 0 must resolve before a row is assembled at all — a symbol with an
invalid forecast produces no causal row rather than a row of zeros.

Symbols are measured concurrently via `errgroup`, each with its own `Pearl`
instance and its own retained history.

## Files

| File        | Responsibility                                                   |
|-------------|------------------------------------------------------------------|
| `solver.go` | Row assembly, ladder evaluation, history retention, publication. |

## Where this connects

Downstream, `logic/graph` reads causal uplift and do-expectation as graph nodes
and infers `supports` / `contradicts` edges by checking whether the causal head
and the resonance forecast **agree directionally**. That comparison is
deliberately direction-only — the two heads score on unrelated scales, and a raw
magnitude comparison would let whichever head has larger units decide the
relation by itself.

Elsewhere in the system, `CausalOutcome.InformedFlow × spread` is what derives
the adverse-selection component of the friction model — the causal head paying
rent beyond observability.

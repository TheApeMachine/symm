# `regulator` — predictive control over trading behavior

The global regulator is an online system-identification loop. It does not score
the current return and turn that score into unrelated settings. It learns the
observed response from controls applied after one account valuation to the next
account valuation.

## Temporal contract

For valuation event `t`, the regulator retains:

- the lagged account return and peak-relative drawdown available at `t`;
- readiness flags that distinguish an uncalibrated standardizer from a real
  zero;
- the exact normalized control vector applied for the interval after `t`.

Only a later valuation revision resolves that input with both
`log(equity[t+1] / equity[t])` and whether the interval carried market exposure.
The outcomes are never present in the input they label. Broker revisions and a
serialized update boundary ensure a valuation is spent at most once. A
separately observed unchanged valuation remains a genuine zero-return outcome;
when it was also flat it is retained as explicit inactivity rather than skipped.
Only repeat delivery of the same revision is ignored.

`learning.ResonanceManifold` performs generative predictive coding over this
state/control vector. Its supervised RLS head provides a Student-t posterior for
the next account log return, and its prequential task skill is measured against
the honest zero-return baseline.

## Live controls

The optimizer changes only settings with a running consumer and defensible
bounds:

| Control | Lower bound | Upper bound | Consumer |
|---|---:|---:|---|
| Entry allocation ceiling | zero | configured ceiling | `strategy.Allocation` |
| Forecast horizon confidence | no-information probability (0.5) | probability one | `logic/resonance` |
| Graph admission boundary | signed reward minimum (-1) | signed reward maximum (1) | `strategy.Planner` |
| Net utility boundary | signed return minimum (-1) | signed return maximum (1) | `strategy.Allocation`, `broker.Desk` |
| Causal search bias | zero | configured baseline | causal MCTS |
| MCTS iterations | one simulation | configured baseline | causal MCTS |
| MCTS exploration | zero | configured baseline | causal MCTS |
| Manifold relaxation | configured minimum | configured maximum | `logic/manifold` |

These controls are normalized only inside those domains. The target remains an
account log return in economic units. MCTS reward, reconstruction error, and
drawdown are not relabeled as one another.

## Identification and optimization

While the wallet has no exposure, the regulator publishes maximum permitted
allocation, the no-information forecast-horizon confidence, the full graph
admission domain, and exact break-even net utility. This directly encodes the
activity objective: an inactive wallet must not become more restrictive before
it has produced an exposed outcome to learn from.

Once exposed, and before prequential skill beats the zero-return baseline, the
regulator applies one-coordinate interventions. Their normalized radius is
`1 / sqrt(movable controls + completed intervention cycles)`, so exploration
shrinks as evidence accumulates without relying on a fixed perturbation size.

After the model becomes predictive, it settles bounded neighboring control
vectors without learning from or advancing temporal state on them and evaluates
both task-head posteriors. Candidate comparison is ordered rather than blended:
a confidently losing wallet is worst, confident inactivity is next, and the
lower Student-t wallet-return quantile decides between candidates in the same
class. Activity evidence breaks an otherwise equal return comparison. The
classification and lower quantiles use the configured optimization confidence.
One persistent intervention per control-dimensional cycle keeps the system
identifiable as market conditions change.

The selected manifold and planner settings are published atomically. The next
planner and manifold cycles read that generation through `system.Config`.

## Scope and limitations

This is an online local optimizer, not evidence of a global optimum or complete
causal identification. Bounded interventions make control response observable,
but the market remains an exogenous source of variation that the current
return/drawdown context cannot fully remove. Repeated outcomes can strengthen a
local response estimate; they cannot turn ordinary market correlation into a
causal guarantee.

The outcome is return per broker valuation interval, not a wall-clock return
rate. Venue latency can therefore change the effective interval. The balance
feed also does not label deposits and withdrawals, so an external cash flow
would be indistinguishable from trading PnL and must not be introduced while
the optimizer is live without first adding cash-flow attribution.

Resonance learning rate and stop geometry are not currently optimized. The live
resonance solvers do not expose a safe retained-
state actuator for their learning pace, stop geometry has a separate execution
contract. Adding either requires a real live consumer and an explicit domain
before it becomes another control coordinate.

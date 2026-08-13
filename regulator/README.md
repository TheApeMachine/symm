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

Only a later valuation revision resolves that input with
`log(equity[t+1] / equity[t])`. The outcome is never present in the input it
labels. Broker revisions and a serialized update boundary ensure a valuation is
spent at most once. A separately observed unchanged valuation remains a genuine
zero-return outcome; only repeat delivery of the same revision is ignored.

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
| Forecast confidence gate | no-information probability (0.5) | probability one | `strategy.Planner` |
| Causal search bias | zero | configured baseline | causal MCTS |
| MCTS iterations | one simulation | configured baseline | causal MCTS |
| MCTS exploration | zero | configured baseline | causal MCTS |
| Manifold relaxation | configured minimum | configured maximum | `logic/manifold` |

These controls are normalized only inside those domains. The target remains an
account log return in economic units. MCTS reward, reconstruction error, and
drawdown are not relabeled as one another.

## Identification and optimization

Before prequential skill beats the zero-return baseline, the regulator applies
one-coordinate interventions. Their normalized radius is
`1 / sqrt(movable controls + completed intervention cycles)`, so exploration
shrinks as evidence accumulates without relying on a fixed perturbation size.

After the model becomes predictive, it settles bounded neighboring control
vectors without learning from or advancing temporal state on them and evaluates
their task-head posterior. It selects the configured upper Student-t quantile,
which balances predicted wallet return with uncertainty about settings that
have not been sufficiently tested.
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

Resonance learning rate, stop geometry, and the MCTS utility threshold are not
currently optimized. The live resonance solvers do not expose a safe retained-
state actuator for their learning pace, stop geometry has a separate execution
contract, and no stationary configured domain yet exists for graph reward.
Adding any of them requires a real live consumer and an explicit domain before
it becomes another control coordinate.

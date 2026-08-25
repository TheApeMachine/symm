# `logic/resonance` — predictive coding over the field

> The market is not what happened. It is the difference between what happened
> and what you expected.

## What this package is

Resonance is a **predictive coding** stage. The idea comes from computational
neuroscience — Rao & Ballard's hierarchical model of cortex, later generalized as
the free-energy principle: a perceptual system does not passively encode input.
It maintains a generative model, continuously predicts its own input from the
level above, and propagates only the **prediction error** upward. Perception is
the process of settling that error to a minimum.

Applied to a market, this reframes the question. The raw feed is not the signal.
The signal is *surprise* — where the market did something the model's settled
expectation did not account for.

The engine is `learning.ResonanceManifold` in
[`nomagique/learning`](../../../nomagique/learning). This package is the market
adapter around it: it decides what counts as an observation, keeps one model per
symbol, records resolved forecast skill, and turns the settled latent state into
a forecast the rest of the system can act on.

## The loop

```
measurements (whichever signals observed this event)
        │
        │  extractFeatures       →  source:symbol:metric  →  normalized value
        ▼
   standardize        (adaptive.Standardizer, per feature)
        │
        │  informative? ──no──►  publish "not ready", wait
        ▼
   latest scoreable feature state
        │  (stable ordering — a vector, not a map)
        │
        ▼
   ResonanceManifold.Settle    ──►  latent state z, layer stack, Surprise
        │
        ├──►  recursive least squares →  fit direction lean + predictive distribution
        ├──►  Student-t direction p   →  confidence in the up/down call
        ├──►  direction-skill ledger  →  how far the model will stand behind a call
        └──►  ResonanceReturnForecast →  Call, Horizon, distribution (not a return)
                                             │
                                             ▼
                              thesis.Resonance[symbol]
```

## Three heads, one manifold

The manifold settles a latent state that serves three purposes at once:

1. **Reconstruction (bottom-up).** How well the generative model explains the
   current observation. Its residual is **Surprise**.
2. **Latent state (z).** The settled compressed representation. Published as a
   cross-section: every symbol's embedding plotted together is a cloud showing
   where symbols sit *relative to one another*, which one point cannot show.
3. **Supervised direction head (V).** One square-root recursive least squares
   row per forward horizon, each trained on the realized direction of the
   cumulative move over its own horizon — row `h` on the sign of the move from
   the issue mark to the mark `h` ticks later, deadbanded by the symbol's own
   recent step noise. The published claim is a call (−1, 0, +1) over the
   supported horizon: the supported reach is the longest contiguous prefix of
   horizon rows whose direction calls beat the zero-prediction baseline at the
   regulated confidence, and defaults to a single tick before any row has
   resolved evidence. Causal and manifold remain the only priced return
   sources downstream.

## Learning is deferred, and that's the point

`learnReturn` is where the supervised head is trained, and it cannot run at
observation time — the target does not exist yet.

The solver retains each issued feature vector and the mark observed at that
analysis epoch. On a **later** ticker epoch with a new mark, it computes
`target = log(mark_now / mark_pending)`. Producer rows can arrive between ticker
epochs and therefore carry an older producer tick; the analysis epoch is the
prediction clock, while the latest FIFO mark is its ground truth. Before the
target is learned, the strictly prior prediction is scored against a zero-return
baseline. Only then does recursive least squares observe the target. A sample is
only spent once the future has actually arrived, and a target can never certify
the prediction that was fitted with it.

The same state is used at publication and resolution: settled `z_t` is the
readout every horizon row is evaluated against. Element `k` of the forward
curve is row `k+1`'s prediction of the cumulative move over the next `k+1`
ticks from the current state — a genuine multi-horizon forecast in which every
element is its own supervised head, not a trajectory through imagined states.
The temporal prior `A` still drives the contraction envelope and the layer
predictions, but the task curve needs no roll-forward: each element is
supervised against its own delayed cumulative target.

Non-positive prices and non-finite returns drop the sample explicitly rather than
poisoning the head with a synthetic zero.

## Readiness: the distinction that matters most

**A zero reading and an absent reading are not the same thing, and conflating
them silently corrupts the model.**

`adaptive.Standardizer` reports whether prior spread makes its current score an
actual measurement. During warm-up it answers zero *without readiness*. But a
ready observation sitting exactly at the learned mean is *also* legitimately
zero. Same number, opposite meanings.

Settling on an unready vector drives the latent state to the origin and turns an
absent observation into a structural zero. Learning from it would spend a resolved
return sample teaching the return head that no input predicts a real move. That is
wrong about a stage that has simply not seen a feature move yet, so when
`informative` is false the reading says so and waits.

This resolves as soon as *any* feature varies, because the standardizer scores
against its own estimate precision rather than waiting out a fixed sample count.
An early reading is small because the scale behind it is uncertain, and it grows
into a full z-score as the moments settle. (A `defaultFeatureWarmup = 32` once
existed here; deleting it cost 30 samples of latency and bought nothing.)

The same logic governs the feature *schema*: if a symbol's model has already
learned against a fixed input schema and an upstream measurement goes missing,
that symbol sits the round out. It must not corrupt another symbol's pass or turn
a gap into a synthetic zero.

## Adaptation with an owned objective

The return learner, confidence, and horizon each adapt from evidence that belongs
to their own objective:

| Quantity                 | Derived by                        | Meaning                                                                        |
|--------------------------|-----------------------------------|--------------------------------------------------------------------------------|
| **Return-head gain**     | square-root recursive least squares | Latent design covariance determines how much each resolved target can teach. |
| **Forecast confidence**  | RLS Student-t posterior predictive | Probability that the current resolved return has the point forecast's direction. |
| **Skill evidence**       | Beta posterior over prequential wins | Probability that prior forecasts beat zero return more than half the time. |
| **Horizon**              | longest contiguous skill prefix | Forecast only as far as the per-horizon rows whose prequential calls beat the zero-prediction baseline; a single tick before any row has resolved evidence. |

There is no separate warming state. The head predicts from its prior immediately;
before residual noise is identifiable its direction probability is the symmetric
50% prior, so the adaptive horizon has no supported reach. Each resolved
innovation updates both coefficient uncertainty and observation noise, producing a
magnitude- and design-dependent Student-t probability for the next forecast.

Historical skill is reported separately. Each resolved forecast contributes a win
or loss by comparing squared return error with the zero-return baseline; an exact
tie carries no evidence. That Beta posterior monitors the learner but is never
presented as confidence in the current price move.

The manifold's generative alpha remains its configured base pace. The old global
pace controller was removed from this adapter because it tuned that alpha from
reconstruction surprise while also moving unrelated generative, recognition,
temporal, precision, regularization, and task-head updates. That was neither a
return-error optimizer nor an identifiable learning-rate controller. The return
head now adapts only the gain that minimizes its own resolved prediction error.

The horizon logic is worth reading closely (`horizon()`): it grows only as
resolved return samples accumulate, then **contracts to the confidence-supported
path length**. If confidence only supports a shorter reach than the nominal
horizon, the horizon is recomputed at the supported confidence rather than
published optimistically.

## Per-symbol, but published as a population

Each symbol gets its own `symbolState` — its own manifold, recursive return
learner, skill posterior, and per-feature standardizers. Symbols do not share a
model.

But publication is deliberately *not* per-symbol-on-demand. Every carrier settled
this round is published, because the latent manifold is a cross-section. Only the
focused row carries the full latent vector and layer stack; every other row
carries its embedding and scalars — which is exactly what the cloud plots.

Symbols are settled concurrently via `errgroup`.

## Files

| File        | Responsibility                                                |
|-------------|---------------------------------------------------------------|
| `solver.go` | Feature extraction, settling, learning, horizon, publication. |
| `state.go`  | Per-symbol state and the nomagique primitives behind it.      |

## Gotchas

- **Held positions drop frames.** A held position gets a `Momentum()` score only
  about half the time: nil manifold/resonance frames on `price_zero` /
  `stale_batch` / `stale_source` stages starve the momentum-decay exit.
  Allow-stage signals are healthy — reject-stage zeros are not a dead pipeline.
- **Never feed Uncertainty in as a price fraction.** It is not one.
- **Confidence ≠ hypothesis separation.** They are distinct per signal, and collapsing them hides
  degenerate signals behind clamping.

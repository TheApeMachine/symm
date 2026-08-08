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
symbol, paces learning, and turns the settled latent state into a forecast the
rest of the system can act on.

## The loop

```
measurements (every signal, this tick)
        │
        │  extractFeatures       →  source:symbol:metric  →  normalized value
        ▼
   standardize        (adaptive.Standardizer, per feature)
        │
        │  informative? ──no──►  publish "not ready", wait
        ▼
   FeatureExtractor   (stable ordering — a vector, not a map)
        │
        ▼
   ResonanceManifold.Settle    ──►  latent state z, layer stack, Surprise
        │
        ├──►  PaceController         →  adapt α (learning rate)
        ├──►  probability.Calibrator →  confidence
        ├──►  DynamicHorizon         →  how far ahead is supported
        └──►  forward rollout        →  ResonanceForecast
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
3. **Supervised return head (V).** Predicts forward log return over an adaptive
   horizon. This is the head that produces an actionable number, and it feeds a
   `predictive_forecast_negative` gate rule downstream.

## Learning is deferred, and that's the point

`learnReturn` is where the supervised head is trained, and it cannot run at
observation time — the target does not exist yet.

The solver holds `pendingInput` (the feature vector) and `pendingMid` (the
midpoint at that moment). On a **later** tick with a new midpoint, it computes
`target = log(mid_now / mid_pending)`, re-settles the stored input, and learns
against that realized return. A sample is only spent once the future has actually
arrived.

Non-positive prices and non-finite returns drop the sample explicitly rather than
poisoning the head with a synthetic zero.

## Readiness: the distinction that matters most

**A zero reading and an absent reading are not the same thing, and conflating
them silently corrupts the model.**

`adaptive.Standardizer` reports whether prior spread makes its current score an
actual measurement. During warm-up it answers zero *without readiness*. But a
ready observation sitting exactly at the learned mean is *also* legitimately
zero. Same number, opposite meanings.

Settling on an unready vector drives the latent state to the origin, which zeroes
rollout retention and publishes the forecast as invalid. Worse, learning from it
spends a resolved return sample teaching the return head that no input predicts a
real move. Both outcomes are wrong about a stage that has simply not seen a
feature move yet — so when `informative` is false the reading says so and waits.

This resolves as soon as *any* feature varies, because the standardizer scores
against its own estimate precision rather than waiting out a fixed sample count.
An early reading is small because the scale behind it is uncertain, and it grows
into a full z-score as the moments settle. (A `defaultFeatureWarmup = 32` once
existed here; deleting it cost 30 samples of latency and bought nothing.)

The same logic governs the feature *schema*: if a symbol's model has already
learned against a fixed input schema and an upstream measurement goes missing,
that symbol sits the round out. It must not corrupt another symbol's pass or turn
a gap into a synthetic zero.

## Adaptive everything

Three quantities that most systems hardcode are derived here:

| Quantity              | Derived by                | Why                                                                                           |
|-----------------------|---------------------------|-----------------------------------------------------------------------------------------------|
| **α** (learning rate) | `learning.PaceController` | A fixed rate is either too slow to track a regime change or too fast to keep what it learned. |
| **Confidence**        | `probability.Calibrator`  | Raw model output is not a probability. Calibration makes it one.                              |
| **Horizon**           | `manifold.DynamicHorizon` | Forecast only as far as resolved samples support.                                             |

The horizon logic is worth reading closely (`horizon()`): it grows only as
resolved return samples accumulate, then **contracts to the confidence-supported
path length**. If confidence only supports a shorter reach than the nominal
horizon, the horizon is recomputed at the supported confidence rather than
published optimistically.

## Per-symbol, but published as a population

Each symbol gets its own `symbolState` — its own manifold, pace controller,
calibrator, and per-feature standardizers. Symbols do not share a model.

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
- **Confidence ≠ SNR.** They are distinct per signal, and collapsing them hides
  degenerate signals behind clamping.

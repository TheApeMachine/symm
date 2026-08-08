# `signal/hawkes` — does a trade excite more trades?

> One trade is a data point. A trade that makes the next trade more likely is
> the market telling you something is feeding on itself.

## What this package is

Hawkes measures **self- and cross-excitation** in the buy/sell trade-arrival
process — whether a trade of one side makes a trade (of either side) more
likely in the moments after it, beyond what a constant background rate would
predict. It is the order-flow analogue of an aftershock model: an arrival is not
just a data point, it is a small, decaying boost to the near-future rate of
arrivals.

This package is the market adapter. It turns raw Kraken trades into a bounded,
per-symbol arrival stream and hands them to
[`nomagique/algorithm/excitation`](https://github.com/TheApeMachine/nomagique/tree/b07bb443d986395a2ab11f7e9969b3d2fe55299a/algorithm/excitation),
which owns the numerical estimator
([`nomagique/hawkes`](https://github.com/TheApeMachine/nomagique/tree/b07bb443d986395a2ab11f7e9969b3d2fe55299a/hawkes)). This package does not
interpret the fit — no category, no trading gate. It emits parameters and
statistical readiness; `logic/category` decides what a given reading means.

## The model

A bivariate exponential-kernel Hawkes process models the conditional intensity
of buy arrivals (x) and sell arrivals (y) as

```
λx(t) = μx + Σ over past buys  AlphaXX·exp(-β(t-ti))
           + Σ over past sells AlphaYX·exp(-β(t-ti))

λy(t) = μy + Σ over past sells AlphaYY·exp(-β(t-ti))
           + Σ over past buys  AlphaXY·exp(-β(t-ti))
```

- **μx, μy** — the baseline ("immigrant") rate: arrivals that would happen
  anyway, independent of recent history.
- **AlphaXX, AlphaYY** — self-excitation: does a buy make the next buy more
  likely (momentum-like), does a sell make the next sell more likely.
- **AlphaXY, AlphaYX** — cross-excitation: does a sell make the next buy more
  likely (and vice versa) — the signature of one side reacting to the other.
- **β** — the shared decay rate. Every excitation term decays at the same
  clock; only its size differs by direction. `1/β` is the memory of the kernel
  — how long one arrival's influence persists.

A trade is one "arrival," marked buy or sell. The estimator fits all seven
parameters (μx, μy, AlphaXX, AlphaXY, AlphaYX, AlphaYY, β) by maximizing the
Hawkes log-likelihood over the observed arrival stream
(`hawkes.NewBivariateEstimator`), constrained to the region where the process
is well-defined (nonnegative rates, sensible decay).

## Why this needs two readiness thresholds, not one

A Hawkes fit is not identifiable from a handful of trades — with too few
arrivals the likelihood surface is flat and any decay/amplitude combination
fits about as well as any other. So the pipeline has two stages with two
different minimum-event bars, and it is important that a reading from the
first stage is never confused for a reading from the second:

1. **Empirical rate** (`Readiness.Intensity`). As soon as there is a positive
   observation span, `BuyArrivalRate`/`SellArrivalRate` are just
   `count / duration`. This is real, but it says nothing about excitation —
   two trades an hour apart and two trades a millisecond apart produce the
   same rate.
2. **Fitted kernel** (`Readiness.HawkesFit`). Once enough events exist per
   side (`FitContext.MinPerSide`, `MinFitEvents` — both derived from the
   observed event count, not fixed constants — see
   `arrivalTune.minFitEvents`/`minEventsPerSide` in
   [`nomagique/hawkes/fit_context.go`](../../../nomagique/hawkes/fit_context.go)),
   the full bivariate estimator runs and Excitation Amplitude, Decay Rate,
   Spectral Radius, and the offspring metrics become meaningful.

Before a fit exists, every metric that depends on one is simply absent from
`Measurement.Metrics` — not zero, not omitted-and-implied-zero, *absent*. A
metric that has not been measured yet must stay distinguishable from one
measured at zero excitation, or a category weight matrix downstream would read
"no excitation" where it should read "no opinion yet."

## Why the fit is compared against two simpler models

A raw Hawkes fit is not trusted on its own. `computeFit`
([`nomagique/algorithm/excitation/symbol_fit.go`](../../../nomagique/algorithm/excitation/symbol_fit.go))
also fits a **self-only restricted model** (`AlphaXY = AlphaYX = 0` — no
cross-excitation allowed) and compares its likelihood against the full fit. If
the restricted model scores at least as well, the full fit is discarded in
favor of it — cross-excitation is not reported unless it earns its extra
parameters. `MetricCrossSelfDelta` is exactly this comparison, published so
the reading exposes its own justification rather than asserting cross-talk
that a simpler model already explains.

Separately, every published fit is compared against a **no-excitation Poisson
baseline** fit from the same context (`FitContext.PoissonFit`) —
`MetricHawkesPoissonDelta`. This is the excitation question in its rawest
form: is a self-exciting process a better explanation for these arrivals than
plain independent noise at all?

## Fits are refit only when evidence has actually moved

The estimator does not refit on every tick. A `schedule`
([`nomagique/algorithm/excitation/schedule.go`](../../../nomagique/algorithm/excitation/schedule.go))
tracks how many arrivals have changed since the last fit and only requests a
new one once enough new evidence has accumulated (`schedule.Ready()`), running
asynchronously so a slow optimization never blocks measurement. Between
refits, the retained parameters are re-evaluated against the *current*
arrival stream — `MetricConditionalIntensity` moves every tick as the kernel's
memory of recent arrivals decays, even though the fitted parameters
themselves are unchanged (`Readiness.ModelUpdated` distinguishes a tick that
published a new parameter epoch from one that only advanced the clock on the
old one).

## Every metric this package produces

All metrics are keyed `type:side` via `types.MetricKey`. `Raw` is the natural
unit; `Normalized` is `nil` when the reading is undefined for the current
readiness state — see the `normalized*` helpers in
[`signal.go`](signal.go) for exactly which comparison each metric uses.

### Available once `Readiness.Intensity` is true

| Metric         | Side       | Meaning                                                                                                                                                                                                                                                                                                                                                                                                                         |
|----------------|------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `event_count`  | none       | Total marked arrivals seen in the observation window. `Normalized` is this count as a share of the minimum event count a fit would need — a running "how close to fit-ready" gauge.                                                                                                                                                                                                                                             |
| `event_count`  | buy / sell | Arrivals on one side. `Normalized` is that side's share of total events — order-flow imbalance in raw counts.                                                                                                                                                                                                                                                                                                                   |
| `arrival_rate` | buy / sell | Empirical rate, `count / span`. Before a fit, `Normalized` is this side's share of the *combined* marked rate (both sides pooled — the only shared baseline available pre-fit). After a fit, `Normalized` switches to `normalizedExcess`: rate expressed as baselines *above* that side's own fitted immigrant rate (μx or μy) — i.e. how much of the observed rate the kernel attributes to excitation rather than background. |

### Available once `Readiness.HawkesFit` is true

| Metric                            | Side                                   | Meaning                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
|-----------------------------------|----------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `conditional_intensity`           | buy / sell                             | λx(t) / λy(t) evaluated at the measurement instant — the model's current instantaneous arrival rate, baseline plus everything still-decaying excitation contributes. `Normalized` is this rate expressed as multiples of that side's baseline above it (`normalizedExcess` against μx/μy) — nonnegative by the kernel's own nonnegativity constraint.                                                                                                                                                                                                    |
| `baseline_intensity`              | buy / sell                             | μx / μy — the fitted background rate, independent of recent history. `Normalized` is this baseline's share of the combined immigrant rate μx+μy — which side the "even with no excitation" flow favors.                                                                                                                                                                                                                                                                                                                                                  |
| `excitation_amplitude`            | buy→buy, sell→buy, buy→sell, sell→sell | AlphaXX, AlphaYX, AlphaXY, AlphaYY respectively — the jump in intensity one arrival of the *from* side contributes to the *to* side's rate. `buy→buy`/`sell→sell` are self-excitation; `sell→buy`/`buy→sell` are cross-excitation. `Normalized` is the amplitude divided by β: the immediate offspring one parent event of that type produces on that target stream, before summing across generations.                                                                                                                                                  |
| `decay_rate`                      | none                                   | β — how fast excitation decays, shared across all four kernel terms. `Normalized` is β relative to the combined immigrant rate — how the kernel's memory clock compares to the background arrival clock.                                                                                                                                                                                                                                                                                                                                                 |
| `kernel_memory`                   | none                                   | `1/β` in seconds — how long one arrival's influence meaningfully persists. `Normalized` is this against the observation horizon — what fraction of the fitting window the kernel "remembers."                                                                                                                                                                                                                                                                                                                                                            |
| `spectral_radius`                 | none                                   | The branching matrix's spectral radius, `ρ(A/β)` — the expected number of direct-and-indirect offspring one arrival produces, summed over all generations of the cascade. `Normalized` is the raw value itself, but **only while `ρ < 1`** (stationary); at or above 1 the expected cascade size diverges and the reading is withheld rather than reported as a number that no longer describes anything observable. This is the single most important number in the fit: it is a literal measurement of how self-reinforcing the current order flow is. |
| `hawkes_poisson_likelihood_delta` | none                                   | Full-fit log-likelihood minus the no-excitation Poisson baseline's log-likelihood, in nats, normalized by event count. Positive and growing means excitation is earning its keep as an explanation; near zero means the arrivals look like plain independent noise.                                                                                                                                                                                                                                                                                      |
| `cross_self_likelihood_delta`     | none                                   | Full bivariate fit's log-likelihood minus the self-only restricted fit's, in nats, normalized by event count. Positive means cross-side excitation is doing real explanatory work beyond what each side's own history already explains; the published fit itself is *forced* to the restricted model whenever this would be negative (see above), so a positive reading here specifically means cross-talk survived that check.                                                                                                                          |
| `immediate_expected_offspring`    | buy / sell                             | Expected number of *direct* child arrivals (one generation deep) that a single parent arrival produces on that side, from `Fit.ImmediateOffspring()`. `Normalized` passes the raw value through, withheld only if negative (a fitted nonnegative kernel cannot produce that, so a negative reading means the fit itself is degenerate).                                                                                                                                                                                                                  |
| `expected_total_descendants`      | buy / sell                             | Expected number of descendants summed across *all* generations (the full cascade, not just the first hop) that a single parent produces on that side, from `Fit.TotalDescendants()`. Diverges as the spectral radius approaches 1; only meaningful together with `spectral_radius`.                                                                                                                                                                                                                                                                      |

### Fields outside `Metrics`

| Field                      | Meaning                                                                                                                                                                                                                        |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ObservedFrom` / `Horizon` | The bounded observation window this measurement was fit or projected over — a sliding window (`FitContext.TradeWindow`), not the full symbol history.                                                                          |
| `Maturity`                 | `min(totalEvents/MinFitEvents, buyEvents/MinPerSide, sellEvents/MinPerSide)`, clamped to `[0,1]`. How close the current window is to having enough events for a trustworthy fit — `1` once a fit is live, partial before that. |
| `Arrivals`                 | The marked buy/sell timestamps carried forward so the *next* tick can reconstruct arrival history without re-querying the trade feed — see [`history.go`](history.go).                                                         |

## What this package deliberately does not decide

`Signal` in [`signal.go`](signal.go) states this directly in its doc comment:
these are statistical measurements, not market regimes, and forecast readiness
(`Readiness.Forecast`) requires residual and out-of-sample validation this
package does not perform. A high spectral radius is not "the market is about
to run" — it is "arrivals are currently self-reinforcing, by this much." What
that implies for a position is `logic/category`'s question to answer, weighing
this evidence against everything else measured on the same tick.

## Files

| File         | Responsibility                                                                                  |
|--------------|-------------------------------------------------------------------------------------------------|
| `signal.go`  | `Signal` lifecycle, per-symbol fan-out, measurement projection, normalization.                  |
| `history.go` | Reconstructs each symbol's bounded arrival stream from the prior `Measurement` plus new trades. |

The estimator itself — context sizing, optimization, likelihood comparison,
refit scheduling — lives in `nomagique/algorithm/excitation` and
`nomagique/hawkes`, not here.

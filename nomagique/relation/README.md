# Relation / Predictive Influence Specification

## Status

Normative specification for the `nomagique/relation` layer.

## 1. Purpose

The Relation layer measures whether prior state in one Measurement coordinate improves causal prediction of a later target coordinate.

It produces directed temporal relationship measurements.

It does not claim physical causality.

A Relation answers:

> Did knowing Source history improve prediction of Target beyond Target's own history and explicitly supplied Controls?

## 2. Unit of Relation

A Relation connects metric coordinates, not signal packages.

Correct:

```text
liquidity.ask_depth_divergence
    →
cvd.signed_net_fraction
```

Incorrect:

```text
liquidity
    →
cvd
```

A signal may contain many mathematically distinct coordinates. They MUST NOT be collapsed before Relation learning.

## 3. Explicit Roles

An Influence estimator receives explicit roles:

- `Source`;
- `Target`;
- zero or more `Controls`;
- timestamped observations;
- candidate lag domain.

Roles MUST be wired explicitly.

The estimator MUST NOT infer source, target, or controls from metric names.

## 4. Preferred Input Coordinate

The preferred input is the signed causal standardized residual of a Measurement:

```text
(current - prior_baseline) / prior_noise_scale
```

This preserves direction, historical context, and a comparable numerical scale.

SNR SHOULD NOT be used as the source coordinate because SNR removes sign.

## 5. Event-Time Alignment

The target event defines prediction time.

For target observation at `t`, Source at lag `τ` is the newest valid Source observation available no later than:

```text
t - τ
```

Every Relation MUST preserve Source observation time, Target observation time, Source age, and selected lag.

Future Source observations MUST never be used.

## 6. Positive Temporal Direction

Predictive Influence requires:

```text
lag > 0
```

because Source must precede Target.

Zero-lag dependence is association and belongs to correlation.

A zero-lag association MUST NOT be published as directed Influence.

## 7. Candidate Lag Resolution

Lag is expressed in time, not bars.

Lag resolution SHOULD be derived from observed Source and Target cadence, using the slower typical cadence as the minimum resolvable lag step.

A finite retained history may bound the available lag domain. That is infrastructure provenance, not a claim that the bound is statistically optimal.

Fixed unexplained rules such as `5 bars`, `10 ticks`, or `20 samples` are prohibited as mathematical definitions.

## 8. Restricted Predictor

The restricted predictor uses Target history and explicitly supplied Controls:

```text
TargetLater
    ←
TargetPast + ControlsPast
```

The predictor family MUST be explicit.

The first required implementation is auditable linear regression.

## 9. Full Predictor

The full predictor adds lagged Source:

```text
TargetLater
    ←
TargetPast + ControlsPast + SourcePast
```

Restricted and full predictors MUST be evaluated on the same valid target observations.

## 10. Prequential Evaluation

Prediction error SHOULD be evaluated prequentially:

1. fit/update using observations strictly before current Target;
2. predict current Target;
3. record prediction residual;
4. only then incorporate current observation.

The current Target MUST NOT train the model used to evaluate that same Target.

## 11. Predictive Gain

Let:

```text
Vr = restricted prediction residual variance
Vf = full prediction residual variance
```

The Relation publishes:

```text
PredictiveGain = log(Vr / Vf)
```

Interpretation:

- `0`: Source did not change predictive error;
- positive: Source reduced unexplained Target variation;
- negative: Source worsened predictive performance under the stated evaluation procedure.

No squashing is applied.

No threshold turns PredictiveGain into `edge_present`.

## 12. Influence Coefficient

The full linear model publishes the fitted Source coefficient.

When causal standardized residual coordinates are used, the coefficient is dimensionless.

Its sign MUST be preserved.

The coefficient is not bullish, bearish, confirming, or rejecting.

## 13. Coefficient Uncertainty

When the fit is identifiable, publish:

- coefficient variance;
- coefficient standard error;
- coefficient SNR.

Primary coefficient SNR:

```text
CoefficientSNR = Coefficient² / CoefficientVariance
```

It is non-negative and unbounded.

It is not probability or confidence.

## 14. Conditional Influence

Conditional Influence is preferred when candidate controls are known.

The Source is evaluated after accounting for Target history and supplied Control histories.

The Relation primitive MUST NOT invent semantic controls.

Controls come from an explicit Relation plan or CausalSchema.

## 15. Rank and Identifiability

If the regression design is rank-deficient:

- coefficient is undefined;
- coefficient variance is undefined;
- coefficient SNR is undefined.

The implementation MUST NOT silently:

- add an arbitrary ridge constant;
- drop a coordinate by name;
- fabricate zero coefficients.

Regularized Influence requires a separate explicit model contract, including data-dependent penalty selection.

## 16. Multiple Lag Search

Searching multiple lags is model selection.

The estimator MUST preserve:

- lag candidate count;
- lag search span;
- lag resolution;
- selected lag;
- predictive criterion at selected lag.

It SHOULD retain the lag-response surface when practical.

No significance threshold is embedded in the Relation layer.

## 17. Lag Selection

The primary lag-selection rule is:

```text
selected lag =
candidate lag with best causal prequential predictive performance
```

The selected lag is a measured property of the stated estimator and search domain.

It is not proof of causal transmission delay.

## 18. Effective Support and Maturity

For estimator weights `w`:

```text
N_eff = (sum w)² / sum(w²)
```

and:

```text
Maturity = 0           when N_eff <= 1
Maturity = 1 - 1/N_eff otherwise
```

Maturity measures effective support.

No arbitrary readiness threshold is embedded.

## 19. Output Contract

A Relation output SHOULD preserve:

```text
Source
Target
Controls
From
At
SourceObservedAt
TargetObservedAt
SourceAge
Lag
LagResolution
LagSearchSpan
LagCandidateCount
Coefficient
CoefficientVariance
CoefficientSNR
RestrictedResidualVariance
FullResidualVariance
PredictiveGain
EffectiveSampleCount
Maturity
```

Undefined fields remain undefined.

## 20. No Edge Boolean

The Relation layer MUST NOT emit `edge_present = true/false` from an arbitrary score threshold.

A valid fitted relationship may have zero coefficient or zero PredictiveGain.

Whether a causal query uses a Relation is a separate model-selection question.

## 21. Historical Dynamics

The layer MAY measure event-time change in:

- coefficient;
- PredictiveGain;
- selected lag;
- residual variance.

Historical recurrence MAY be measured using standardized trajectories.

No regime label is emitted.

## 22. Invalid States

The estimator MUST distinguish:

- no Source history;
- no Target history;
- no positive candidate lag;
- no aligned causal rows;
- zero Target variance;
- rank deficiency;
- unavailable residual variance;
- unavailable coefficient uncertainty;
- observed zero coefficient;
- observed zero PredictiveGain.

Invalid is not zero.

## 23. Explicit Non-Claims

A Relation does not prove:

- physical causality;
- information flow;
- manipulation;
- price discovery;
- profitability;
- persistence;
- strategy value.

It measures directed temporal predictive contribution.

## 24. Conformance Checklist

A Relation implementation is non-conformant if it:

1. relates whole signal packages rather than coordinates;
2. uses SNR as a substitute for signed Source state;
3. includes future observations;
4. uses zero-lag association as Influence;
5. chooses lag through unexplained fixed constants;
6. silently regularizes singular fits;
7. thresholds Influence into an edge boolean;
8. deletes low-gain evidence;
9. calls predictive Influence causal;
10. hides Source/Target/lag provenance.

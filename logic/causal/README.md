# Causal Reasoning Model Specification

## Status

Normative specification for converting Measurements and Relation structure into causal and counterfactual estimates.

## 1. Purpose

The Causal layer answers explicit questions such as:

> What outcome should be expected under intervention X, given the observed market and portfolio state?

It consumes:

- Measurement history;
- an explicit `CausalSchema`;
- Influence Graph evidence;
- strategy treatment/action definitions.

It does not consume a handful of opaque semantic scores as a substitute for market state.

## 2. Predictive Influence Is Not Causality

An Influence Graph edge is evidence of temporal predictive structure.

It is not automatically a causal edge.

The Causal layer MUST distinguish:

1. association;
2. predictive temporal Influence;
3. assumed structural parent;
4. identified causal effect.

A causal estimate MUST NOT be produced merely because a Granger-style Influence exists.

## 3. CausalSchema

Every causal model operates under an explicit versioned `CausalSchema`.

The schema defines:

- variable identities;
- variable roles;
- time semantics;
- allowed temporal directions;
- forbidden directions;
- treatment/action variables;
- outcome variables;
- portfolio variables;
- exogenous/context variables;
- candidate market-parent relationships;
- model epoch.

The schema is the wiring contract.

Metric names are not the wiring mechanism.

## 4. Market Variables

Market variables are actual Measurement coordinates.

Examples:

```text
liquidity.ask_depth_divergence
cvd.signed_net_fraction
hawkes.conditional_intensity:buy
derivatives.open_interest_growth_rate
sentiment.breadth
```

A universal semantic frame such as:

```text
flow
coherence
regime
surprise
```

MUST NOT replace these coordinates.

## 5. Temporal Structural Form

The primary representation is a time-sliced structural model.

A variable at time `t` may depend on explicitly allowed variables from earlier causal times.

Future-to-past edges are forbidden.

Lagged Influence edges MAY nominate candidate parents.

Contemporaneous causal edges require separate explicit assumptions.

## 6. Strategy Actions

Strategy actions are explicit intervention variables.

Examples:

- Wait;
- Enter;
- Exit;
- Scale.

An action directly changes only variables the strategy actually controls, such as:

- position;
- inventory;
- cash;
- order state.

An action MUST NOT directly mutate market Measurements unless an explicit market-impact model exists.

Structurally justified:

```text
Enter → Position
```

Forbidden without an impact model:

```text
Enter → MarketPrice
```

## 7. Outcomes

The causal query target MUST be an explicit outcome.

Preferred economic outcomes include:

- future portfolio wealth change;
- realized P&L;
- future log return over an explicit horizon;
- slippage;
- drawdown;
- execution cost.

The target MUST NOT be regime score, opportunity score, confidence, or category ID.

## 8. Observational History

Only real observed rows may fit the causal model.

MCTS simulations MUST NOT enter the observational fit dataset.

Every row MUST preserve:

- schema version;
- model epoch;
- event time;
- coordinate identity;
- variable values;
- missing/undefined status.

Rows from incompatible epochs MUST NOT be silently pooled.

## 9. Matrix Materialization

A numeric matrix MAY be used internally.

Its columns MUST be generated from the explicit CausalSchema.

A universal hard-coded column array is prohibited as the permanent reasoning boundary.

Within one schema version, column identity MUST be stable, explicit, and reversible.

## 10. Query-Local Parent Set

For Target `Y`, the model uses an explicit query-local parent set.

Candidate parents may come from:

- CausalSchema;
- lagged Influence Graph evidence;
- Target's own history;
- explicitly required confounders.

The full Measurement history remains retained when a coordinate is not selected for the current Target.

## 11. No Universal Feature List

The system MUST NOT rely on one forever-fixed list such as:

```text
Flow
LiquidityImpact
Hawkes
Coherence
Regime
Surprise
```

for every causal query.

Different Targets may require different explanatory variables.

Feature selection is part of the explicit query/model contract.

## 12. Identification

A `do(Treatment)` estimate is valid only when the CausalSchema provides a defensible identification strategy.

For backdoor adjustment, the engine MUST preserve:

- Treatment;
- Target;
- adjustment variables;
- identification reasoning.

If a valid adjustment set cannot be established:

```text
IdentificationStatus = NotIdentifiable
```

The engine MUST NOT silently return observational association as a causal effect.

## 13. Treatment Positivity

A Treatment effect cannot be identified where observational history contains no relevant support for that Treatment state under comparable controls.

Unsupported interventions MUST be reported as unsupported.

The engine MUST NOT arbitrarily extrapolate and label the result identified.

## 14. Predictor Family

The first required predictor family is linear regression because coefficients, residuals, and rank are auditable.

A nonlinear predictor MAY be added under its own specification.

Changing predictor family MUST NOT weaken this causal contract.

## 15. Rank and Effective Support

A valid linear fit requires sufficient rank and effective support.

The mathematical requirement depends on:

- fitted parameter count;
- matrix rank;
- effective observations.

A universal arbitrary rule such as `minimumRows = 100` MUST NOT be the sole definition of readiness.

An infrastructure minimum count MAY exist, but causal validity still depends on rank, support, and identification.

## 16. Regularization

No arbitrary ridge constant is allowed.

If regularization is needed, its penalty MUST be chosen by an explicit data-dependent method, such as causal rolling validation or another separately specified criterion.

The selected method and value MUST be recorded in model provenance.

## 17. Causal Estimate Contract

A causal estimate SHOULD preserve:

```text
Target
Treatment
TreatmentLevel
AdjustmentSet
ObservedStateAt
ExpectedOutcome
EffectRelativeToReference
ResidualNoise
StandardError
EffectiveSupport
Maturity
IdentificationStatus
ModelVersion
SchemaVersion
From
At
```

Unavailable uncertainty remains unavailable.

## 18. Counterfactual Contract

A counterfactual answers:

> Given what actually happened, what does this fitted structural model imply under a different intervention?

It MUST preserve:

- factual action;
- counterfactual action;
- factual outcome;
- counterfactual estimate;
- factual residual/noise where required by method;
- uncertainty;
- model/schema version.

## 19. Quality Is Not One Confidence

Causal quality MUST preserve separate concepts:

- IdentificationStatus;
- effective support;
- residual noise;
- parameter uncertainty;
- out-of-sample/prequential diagnostics where available.

A bounded transform such as:

```text
1 / (1 + abs(noise))
```

MUST NOT become a universal trust score unless separately calibrated and specified.

## 20. Maturity

For weighted observational evidence:

```text
N_eff = (sum w)² / sum(w²)
Maturity = 0           when N_eff <= 1
Maturity = 1 - 1/N_eff otherwise
```

Maturity does not override identification.

A mature confounded model remains non-identifiable.

## 21. Influence Graph Use

Influence edges MAY:

- nominate lagged candidate parents;
- provide lag initialization;
- expose predictive structure;
- expose coefficient uncertainty.

They MUST NOT automatically:

- become causal edges;
- determine adjustment sets;
- justify intervention.

Causal assumptions remain explicit.

## 22. Mediation and Double Counting

The Causal model SHOULD represent structural paths rather than independent evidence votes.

If the measured structure is:

```text
Hawkes
   ↓
CVD
   ↓
Price
```

the model SHOULD NOT add separate Hawkes and CVD semantic scores as independent votes for Price.

Mediation is represented by the path.

## 23. Non-Identifiable Is a Valid Result

A non-identifiable query MUST be representable as:

```text
Estimate unavailable:
causal effect not identifiable under current schema/history.
```

The engine MUST NOT:

- substitute correlation;
- substitute predictive Influence;
- return zero;
- reuse an old estimate as current.

## 24. Causal Update Ordering

At event time `t`:

1. materialize current state from observations available at or before `t`;
2. evaluate the existing model;
3. emit current causal estimates;
4. only then update model state with observation `t`.

The current Target MUST NOT train the model used to evaluate itself.

## 25. Model Epoch Changes

A new causal epoch MUST begin when schema, variable meaning, treatment semantics, Target definition, or material market/cohort reference changes.

Historical rows remain accessible but MUST NOT be silently mixed across incompatible epochs.

## 26. Conformance Checklist

The Causal model is non-conformant if it:

1. treats Influence as automatic causality;
2. uses semantic score columns instead of underlying Measurements;
3. has one universal feature set for every Target;
4. cannot return NotIdentifiable;
5. silently substitutes correlation for causal effect;
6. trains on MCTS simulations;
7. lets strategy actions mutate market variables without an explicit impact model;
8. uses arbitrary regularization;
9. hides the adjustment set;
10. loses schema/model provenance.

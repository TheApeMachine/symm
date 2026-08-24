# Global Reasoning Architecture Specification

## Status

Normative architecture contract.

The words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are normative.

## 1. Purpose

Nomagique separates observation, measurement, learned temporal relationships, causal reasoning, strategic search, and operational constraints:

```text
MARKET
  ↓
SIGNALS
  ↓
MEASUREMENTS
  ↓
RELATION
  ↓
INFLUENCE GRAPH
  ↓
CAUSAL MODEL
  ↓
MCTS
  ↓
ACTION
```

Each layer has one responsibility.

## 2. Primary Invariant: Information Preservation

No downstream layer may replace a richer upstream representation with a semantic scalar unless that scalar is itself the mathematically defined answer to the current query.

Legitimate scalar answers include mean, variance, covariance, correlation, expected return, expected P&L, lag, residual variance, regression coefficient, and predictive gain.

Prohibited replacements include market strength, confidence score, regime score, bullishness, coherence score, opportunity score, setup quality, and trend score.

A semantic view MAY be generated for UI or explanation. It MUST NOT become the only retained representation of the underlying facts.

## 3. Preservation Contract

Every valid signal metric MUST remain available in observational history unless it expires under the explicit retention policy.

A metric is not deleted merely because it is not selected for the current causal query, its influence is small, its SNR is low, another metric is more predictive, or a downstream category does not use it.

Feature selection is query-local. Feature selection is not data deletion.

## 4. Layer Responsibilities

### Signal

Signals measure market facts. Signals MUST NOT choose strategy actions, infer intent, assign trading categories, decide which other signals matter, or encode hypotheses into bounded scores.

### Relation

Relation measures temporal predictive dependence between measurement coordinates. Relation MUST NOT claim physical causality, decide actions, create market categories, or delete measurements.

### Influence Graph

The graph stores current and historical measured temporal relationships. It MUST NOT convert all edges into one signal-family score, discard edges because of a value threshold, or call observational temporal dependence causal.

### Causal Model

The causal layer answers explicit causal or counterfactual queries under explicit structural assumptions. It MUST distinguish association, predictive influence, identified causal effect, and non-identifiable causal question.

### MCTS

MCTS searches strategic actions. It MUST consume causal outcome estimates and economic state. It MUST NOT reinterpret raw signal metrics into hand-written evidence gates.

### Operational / Risk Constraints

Hard constraints belong here. Examples include impossible actions, exchange constraints, inventory constraints, and explicit exposure limits.

## 5. Hard Gates

Hard gates are permitted only for physical impossibility, mathematical domain requirements, venue constraints, explicit strategy/risk policy, or estimator identifiability.

Allowed examples:

```text
quantity > 0
bid < ask
elapsed_time > 0
matrix has sufficient rank
position exists before exit
order size <= risk limit
```

Forbidden evidence gates:

```text
if confidence < 0.65: ignore
if hawkes < 0.40: reject
if liquidity_score > 0.70: exit
if regime != TREND: wait
if influence < 0.20: delete edge
```

Valid but uncertain evidence remains evidence. Uncertainty MUST remain in the model rather than becoming an arbitrary present/absent decision.

## 6. Undefined Is Not Zero

Every layer MUST distinguish observed zero, estimated zero, unavailable, mathematically undefined, insufficient support, and non-identifiable.

No missing or undefined value is silently replaced by zero.

## 7. Observational Evidence vs Simulation

Observational market history and simulated strategy trajectories MUST remain separate.

MCTS rollouts MUST NOT be inserted into data used to fit signal baselines, relation models, Influence Graph edges, or causal models.

Simulation cannot become evidence because the strategy generated it.

## 8. Event-Time Causality

At time `t`, only observations at or before `t` may be used. The current observation is evaluated against prior model state before model state updates.

Future observations MUST NOT influence current estimates.

As-of alignment MUST preserve source age and provenance.

## 9. Model Epochs

A new model epoch MUST begin when a material structural contract changes, including metric definitions, causal schema, treatment semantics, cohort definition, relation candidate plan, or market/reference semantics.

Incompatible epochs MUST NOT be silently mixed.

## 10. Schema, Not Name Magic

Reasoning MUST use an explicit schema that binds metric identity, source, symbol, unit, timescale, role, lag, and internal model coordinate.

Names are provenance. Names are not wiring.

No component may infer a causal or mathematical role because a metric contains words such as `flow`, `hawkes`, `liquidity`, `spread`, or `strength`.

## 11. Query-Local Projection

A causal query MAY use only a subset of available coordinates, but the projection MUST be explicit, reversible, provenance-preserving, and query-local.

Unused coordinates remain in history for other queries.

## 12. No Universal Semantic Reasoning Frame

A fixed frame such as:

```text
flow
liquidity_impact
hawkes
coherence
regime
surprise
```

MUST NOT serve as the universal information boundary between Measurements and causal reasoning.

Such variables MAY exist as optional views. They MUST NOT replace the measured coordinates from which they were derived.

## 13. Quality Is Multidimensional

At minimum preserve:

- Maturity: effective estimator support;
- SNR: distinguishability under the relevant noise model;
- model residuals: unexplained error;
- identification status: whether a causal query is justified.

These MUST NOT be fused into one universal `confidence`.

## 14. Reviewability

Every output consumed by strategy MUST be traceable to source Measurements, transformation equations, retained history, relation edges, causal assumptions, model/schema version, and action evaluation.

A reviewer must be able to answer: **Why did this number exist?**

## 15. Prohibited Implementation Patterns

The following are architecture violations:

```go
score := 0.4*flow + 0.3*liquidity + 0.3*hawkes
```

```go
if influence < 0.2 {
    delete(edge)
}
```

```go
if causalEstimateUnavailable {
    useCorrelationInstead()
}
```

```go
history = append(history, rollout)
```

A fallback model is allowed only when it is an explicit named model with explicit provenance. Silent fallback is prohibited.

## 16. Conformance Test

An implementation conforms only if all answers are yes:

1. Are all valid source Measurements retained independently of current strategy use?
2. Are Relation outputs mathematical measurements rather than market judgments?
3. Can low and zero influence remain representable without threshold deletion?
4. Can a causal query return `not identifiable`?
5. Are query feature sets explicit rather than universally hard-coded?
6. Are simulated rows excluded from observational learning?
7. Are strategy actions constrained only by reality/policy, not arbitrary evidence gates?
8. Can the final action be traced back to the exact observational coordinates used?
9. Are undefined values distinct from zero?
10. Can another implementation be rejected solely by comparing it with this contract?

If any answer is no, the reasoning pipeline is non-conformant.

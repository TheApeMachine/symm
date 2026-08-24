# Influence Graph Specification

## Status

Normative specification for the time-indexed observational relation graph.

## 1. Purpose

The Influence Graph stores measured directed temporal relationships between Measurement coordinates.

It is a map of observed predictive structure.

It is not itself a Structural Causal Model.

## 2. Nodes

A node represents one Measurement coordinate identity.

A node MUST preserve:

- metric identity;
- signal source;
- market symbol;
- peer identity when bivariate;
- unit;
- timescale;
- model epoch.

Example:

```text
BTC-PERP / liquidity / ask_depth_divergence
```

The graph MUST NOT collapse a whole signal package into one node unless that package truly emits one scalar phenomenon.

## 3. Edges

An Influence edge is one valid Relation measurement.

An edge MUST preserve:

- Source node;
- Target node;
- lag;
- coefficient;
- PredictiveGain;
- coefficient uncertainty;
- Maturity;
- observation interval;
- estimator provenance.

An edge is a measurement, not a category.

## 4. Time Index

Edges are time-varying.

The graph MUST support:

- current relation state;
- historical edge state;
- model epoch.

A relationship may strengthen, weaken, reverse coefficient sign, or change lag.

Current values MUST NOT erase historical edge state when relation dynamics are required.

## 5. Candidate Relation Plan

The graph builder evaluates an explicit candidate relation plan.

The plan defines structurally eligible Source/Target pairs.

Eligibility MAY depend on:

- symbol scope;
- venue scope;
- explicit cohort/pair configuration;
- standardized-coordinate availability;
- timescale compatibility;
- model epoch.

Eligibility MUST NOT depend on a current evidence score.

Forbidden:

```text
only estimate CVD influence when hawkes > 0.7
```

Allowed:

```text
estimate all configured same-symbol compatible coordinate pairs
```

## 6. No Implicit Peer Discovery

Cross-symbol relations require explicit scope.

The graph MUST NOT choose peers merely because symbols have similar names, moved similarly, are currently correlated, or one is the largest mover.

Peer/cohort construction belongs to an explicit outer contract.

## 7. Candidate, Estimated, Unavailable

The graph SHOULD distinguish:

- `Candidate`: structurally scheduled for estimation;
- `Estimated`: currently has a valid Relation;
- `Unavailable`: candidate exists but its estimator is currently undefined.

Unavailable MUST NOT be treated as "no relationship."

## 8. No Threshold Pruning

The graph MUST NOT delete an edge because:

- PredictiveGain is small;
- coefficient is small;
- coefficient SNR is low;
- Maturity is low.

Low and zero are valid measurements.

If bounded storage requires eviction, eviction MUST follow explicit retention policy rather than a market-value threshold. The candidate plan remains intact so the Relation remains re-estimable.

## 9. Association Edges

Contemporaneous correlation MAY be visualized in the same graph, but it MUST use a distinct type:

```text
Association
```

rather than:

```text
Influence
```

A zero-lag correlation MUST NOT masquerade as directed temporal Influence.

## 10. Lagged Cycles

Lagged cycles are allowed.

Example:

```text
A(t-2) → B(t)
B(t-1) → A(t)
```

This does not imply an instantaneous causal cycle because the edges connect different time slices.

Contemporaneous causal cycles require an explicit causal model and MUST NOT be inferred from the Influence Graph.

## 11. Graph Queries

The graph MUST support explicit queries such as:

- incoming Influences for a Target;
- outgoing Influences from a Source;
- edge history;
- all candidate relations in one symbol;
- cross-symbol relations within an explicit cohort;
- paths between coordinates.

Query results MUST retain the underlying edge measurements.

## 12. Ranking Is Not Deletion

A consumer MAY rank edges by:

- PredictiveGain;
- coefficient SNR;
- Maturity;
- absolute coefficient;
- lag.

Ranking changes presentation/order only.

Lower-ranked edges remain available.

## 13. Signal-Family Rollups

A UI MAY show a rollup such as:

```text
Liquidity → CVD
```

only as a view over coordinate-level edges.

The rollup MUST:

- expose the underlying edges;
- state the aggregation rule;
- remain reversible.

It MUST NOT replace those edges as the input to causal reasoning or MCTS.

## 14. No Graph Confidence

Each edge carries its own quality.

A graph-level summary MAY expose distributions such as median Maturity or number of currently estimable edges.

It MUST NOT create one universal `graph_confidence` that replaces edge-specific quality.

Likewise, edge SNR values MUST NOT simply be summed into market confidence.

## 15. Observational Boundary

An Influence edge:

```text
X → Y
```

means:

> past X improved prediction of later Y under the stated Relation model.

It does not by itself justify an intervention:

```text
do(X=x)
```

The Causal layer owns causal identification.

## 16. Graph-to-Causal Handoff

The graph MAY provide:

- candidate lagged parents;
- temporal ordering;
- predictive coefficients;
- PredictiveGain;
- coefficient uncertainty;
- relation history.

The Causal Model MUST independently preserve:

- structural assumptions;
- forbidden directions;
- treatment semantics;
- adjustment sets;
- identification status.

The graph is evidence for structure, not automatic causal truth.

## 17. Schema Versioning

Every graph snapshot MUST identify:

- node schema version;
- relation-plan version;
- model epoch.

Edges from incompatible schemas MUST NOT be silently merged.

## 18. Persistence and Reconstruction

Persistent graph data SHOULD be sufficient to reconstruct:

- why an edge was attempted;
- exact Source and Target identities;
- searched lag domain;
- current relation statistics;
- relevant relation history;
- estimator version.

A serialized graph MUST NOT consist only of an anonymous numeric matrix.

## 19. Missing and Zero States

The graph MUST distinguish:

- candidate but unavailable;
- valid zero coefficient;
- valid zero PredictiveGain;
- stale Relation;
- incompatible epoch;
- missing node;
- estimator failure.

No missing edge value is replaced by zero.

## 20. Conformance Checklist

The Influence Graph is non-conformant if it:

1. collapses signal packages to one semantic node each;
2. deletes edges below a threshold;
3. treats correlation as directed Influence;
4. treats Influence as causal truth;
5. loses lag provenance;
6. loses schema/model epoch;
7. uses graph confidence as a replacement for edge quality;
8. silently invents peers;
9. discards underlying edges after a family rollup;
10. cannot distinguish unavailable from measured zero.

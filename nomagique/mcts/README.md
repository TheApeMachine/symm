# MCTS Causal Strategy Specification

## Status

Normative specification for strategic Monte Carlo Tree Search over the Nomagique causal market model.

## 1. Purpose

MCTS searches feasible strategic actions using causal outcome estimates.

Its job is:

> Given the currently observed market state, portfolio state, and causal model, which feasible action has the best expected economic consequence?

MCTS does not decide which Measurements are "real enough."

MCTS does not convert signal metrics into evidence gates.

## 2. Inputs

MCTS receives:

1. current observational market state;
2. Causal Model and schema;
3. portfolio state;
4. feasible-action contract;
5. explicit economic objective;
6. observational-history reference;
7. model uncertainty.

It MUST NOT receive only semantic summaries such as `flow score`, `regime`, `coherence`, and `confidence` as substitutes for the underlying state.

## 3. Actions

Initial strategic interventions may include:

- `Wait`;
- `Enter`;
- `Exit`;
- `Scale`.

Actions are interventions.

They are not graph-node selections.

## 4. Feasible Action Constraints

Hard action constraints are allowed only when they represent reality or explicit policy.

Examples:

```text
flat position:
    cannot Exit

position at maximum exposure:
    cannot Scale

venue minimum quantity not met:
    cannot submit order

risk limit exceeded:
    action unavailable
```

These are feasibility constraints, not evidence gates.

## 5. Forbidden Strategy Gates

The following are prohibited:

```text
if confidence < 0.65:
    Wait

if hawkes < 0.4:
    cannot Enter

if regime != Trend:
    cannot Scale

if influence < 0.2:
    ignore variable
```

Valid but uncertain evidence remains in the Causal Model.

Uncertainty affects outcome estimation rather than action feasibility.

## 6. State

An MCTS state contains:

- portfolio/account state;
- strategy horizon;
- reference to the causal market state;
- causal model version;
- observational provenance.

The state MUST NOT be a lossy semantic summary of the market.

## 7. Market Evolution

Rollout market evolution comes from the explicit causal/predictive transition model.

A strategy action affects market variables only through explicitly modeled causal mechanisms.

Without an explicit market-impact model:

```text
Action → PortfolioState
```

but not:

```text
Action → MarketPrice
```

MarketPrice may affect portfolio P&L. That is a different causal direction.

## 8. Simulation Is Not Observation

MCTS-generated states are hypothetical.

They MUST NOT be inserted into:

- signal baselines;
- Relation history;
- Influence Graph fitting data;
- causal observational tables.

Simulated trajectories remain inside search.

## 9. Economic Reward

The default reward MUST be tied to an actual economic quantity.

Preferred base reward:

```text
change in net portfolio wealth
after explicit transaction costs
```

or an explicitly specified log-wealth form.

Reward MUST NOT be a weighted sum of signal scores.

Forbidden:

```text
0.4 * flow_score
+ 0.3 * hawkes_score
+ 0.3 * liquidity_score
```

Measurements describe state. They are not the objective.

## 10. Costs

Known costs SHOULD enter reward directly in their real units.

Examples:

- exchange fees;
- spread crossing;
- modeled slippage;
- funding when relevant.

They MUST NOT be converted into arbitrary penalty scores.

## 11. Risk

Hard risk policies belong in action feasibility.

Examples:

- maximum gross exposure;
- maximum position size;
- prohibited instrument;
- explicit loss limit.

If a strategy uses risk-adjusted utility beyond hard constraints, that utility function MUST be explicitly specified as strategy policy.

Its parameters MUST NOT be disguised as market mathematics.

## 12. Causal Action Evaluation

For each candidate action, MCTS requests a causal/counterfactual outcome estimate.

The estimate SHOULD preserve:

- expected economic outcome;
- uncertainty;
- IdentificationStatus;
- model support;
- model/schema version.

An action whose required causal outcome is non-identifiable MUST NOT receive a fabricated zero reward.

## 13. Undefined Action Estimates

If an action outcome cannot be validly estimated:

```text
ActionEstimate = Undefined
```

The search MUST preserve that status.

It MUST NOT silently:

- reuse an old estimate;
- replace it with correlation;
- replace it with zero;
- inject an arbitrary pessimistic penalty.

If no feasible action has an estimable objective, search returns:

```text
DecisionUnavailable
```

Operational policy may decide safe external behavior. That policy is outside the causal evidence model.

## 14. Uncertainty

Uncertainty SHOULD influence search continuously through the action-value estimator.

It MUST NOT be converted into `trusted / untrusted` with an arbitrary threshold.

The tree SHOULD preserve:

- mean estimated reward;
- reward variance or standard error;
- visit count;
- causal-estimate support.

## 15. Exploration

Exploration MUST use an explicit search rule.

Any exploration parameter MUST be:

- theoretically derived for the selected rule;
- normalized to the economic reward scale;
- or explicitly declared strategy policy.

An unexplained magic exploration constant is prohibited.

An uncertainty-aware UCB-style rule is preferred because exploration can be tied to observed reward uncertainty rather than market evidence scores.

## 16. Horizon

The rollout horizon MUST have explicit meaning.

Allowed sources include:

- strategy holding-horizon policy;
- causal model predictive support;
- contractual execution horizon;
- portfolio lifecycle.

A horizon MUST NOT be selected merely because a convenient fixed integer exists.

If an external maximum horizon is configured, it is strategy policy and MUST be named as such.

## 17. Tree Backpropagation

Backpropagation aggregates economic rollout outcomes.

It MUST NOT inject unrelated signal scores into node reward after causal simulation.

Causal and counterfactual provenance SHOULD remain attached to node traces.

## 18. No Evidence Voting

This reasoning pattern is prohibited:

```text
Liquidity says Enter
Hawkes says Enter
CVD says Enter
3 votes > 2 votes
therefore Enter
```

The Causal Model represents dependencies between Measurements.

MCTS evaluates action consequences under that model.

## 19. No Double Counting

MCTS MUST NOT independently reward multiple Measurements representing one mediated causal path.

Example:

```text
Hawkes → CVD → Price
```

is not:

```text
Hawkes reward + CVD reward + Price reward
```

unless the economic objective explicitly contains three independent economic quantities.

## 20. Action Selection

The selected action is the feasible action with the best search value under the specified economic objective and search rule.

The final result SHOULD preserve:

```text
SelectedAction
ExpectedEconomicOutcome
OutcomeUncertainty
Visits
AlternativeActions
CausalModelVersion
SchemaVersion
IdentificationStatus
Trace
```

A bare action without provenance is insufficient.

## 21. Traceability

For every selected action, a reviewer MUST be able to inspect:

1. current measured state;
2. causal variables used;
3. relevant Influence Graph edges;
4. adjustment set;
5. causal/counterfactual outcome estimate;
6. economic reward calculation;
7. MCTS search statistics;
8. rejected actions and whether they were infeasible or undefined.

## 22. No Final Hard Gate

After MCTS produces its action-value comparison, there MUST NOT be another hidden semantic gate such as:

```text
if final_confidence < 0.7:
    force Wait
```

Any post-search override must be an explicit operational/risk constraint with documented provenance.

## 23. Position-State Constraints

Position logic is a legitimate hard constraint.

Example:

```text
flat:
    Wait
    Enter

positioned:
    Exit
    Wait
    Scale
```

This describes physically meaningful interventions. It is not evidence filtering.

## 24. Model Readiness

MCTS MUST NOT use one arbitrary row count as the sole definition of causal readiness.

Readiness depends on the requested estimate:

- identification;
- rank;
- effective support;
- model uncertainty;
- available outcome horizon.

A row count may be an infrastructure requirement but is not sufficient by itself.

## 25. Strategy Categories

A strategy MAY produce semantic explanations after action selection.

For example:

```text
waiting because estimated net wealth change was lower than alternatives
```

It MUST NOT reduce the market into a category and then select actions solely from a category lookup.

## 26. Conformance Checklist

MCTS is non-conformant if it:

1. receives only compressed semantic signal scores;
2. hard-gates valid evidence using arbitrary thresholds;
3. trains causal models on rollouts;
4. rewards signal scores instead of economic outcomes;
5. lets actions mutate market state without an explicit impact model;
6. hides non-identifiable estimates by returning zero;
7. uses undocumented exploration constants;
8. performs evidence voting;
9. applies a hidden final confidence gate;
10. cannot trace the selected action back to Causal estimates and Measurements.

# Thesis Pipeline and Lifecycle

## Objective

Build one traceable pipeline that turns raw market events into measurements,
shapes those measurements into contextual hypotheses, evaluates the utility of
possible portfolio actions, records the resulting trade lifecycle on the same
Thesis, and performs a constrained PostMortem after the lifecycle completes.

The package boundaries are:

1. `signal` conditions raw market data into typed numerical measurements.
2. `logic` aligns and composes measurements, preserves conflict, and assigns
   contextual categories and hypotheses.
3. `strategy` forecasts outcomes, evaluates action-specific utility, and appends
   decisions to the Thesis.
4. `broker` validates feasibility, executes decisions without reinterpreting
   them, and appends immutable execution and position observations.
5. `PostMortem` evaluates the completed causal record and emits findings and
   candidate adjustments. It does not directly mutate the running system.

The Thesis is the in-memory causal case record shared by these stages. No
persistence identifier or parallel record is required while the lifecycle is
in-process.

## Non-negotiable distinctions

### Measurements are not interpretations

A measurement is a numerical estimate of a defined market property. Examples
include signed return velocity, trade intensity, cancellation intensity,
pressure gradient, spread, or order-flow imbalance.

A signal must not turn a measurement into a trade decision or market narrative.
Names such as `VerticalIgnition`, `Frenzy`, `SpoofTrap`, and `Exhaustion` are
contextual interpretations and belong in `logic`.

### Categories are not decisions

A category is a dimensionality-reduced description of a composed market state.
It supplies precomputed context to forecasting and strategy. It does not imply
an action.

The same category can support different actions under different prices,
liquidity, portfolio exposure, uncertainty, and execution costs.

### Conflict is evidence

Conflicting measurements must not be averaged away. Logic must determine
whether the apparent conflict comes from:

- Different subjects.
- Different sides.
- Different horizons.
- Different causal mechanisms.
- Staleness.
- Redundant estimators.
- Statistically incompatible forecasts of the same outcome.

Only the final case demonstrates calibration conflict. Other cases may jointly
describe a valid but nuanced market state.

### Decisions are not executions

Strategy records the selected action and its utility. Broker determines whether
that action remains feasible and records what actually happened. Broker must
not silently replace or reinterpret the requested action.

### Profit is not proof of correctness

A profitable trade may have been an unjustified decision that received a
favorable realization. A losing trade may have been a calibrated positive-value
decision whose adverse outcome was within the predicted distribution.

PostMortem must evaluate measurements, reasoning, forecasts, decisions,
execution, and position management separately.

## End-to-end data flow

```text
Kraken market events
    -> signal measurements
    -> logic alignment and composition
    -> contextual categories and hypotheses
    -> forecasts
    -> strategy utility alternatives
    -> decision appended to Thesis
    -> broker intent and execution
    -> position and market observations appended to Thesis
    -> continuation, exit, and reversal evaluation
    -> closing execution
    -> post-exit observation tail
    -> PostMortem findings
    -> aggregate validation
    -> versioned candidate adjustment
```

## Shared domain contracts

Shared contracts should live in `types` or another focused domain package. The
package must contain contracts only, not signal, logic, strategy, or broker
implementation.

This corrects the current dependency in which `logic` imports
`strategy.Thesis` merely to transport evidence.

## Measurement contract

Replace the category-centered `types.Measurement` with a typed numerical
contract. A measurement must preserve both its native value and any normalized
representation.

Conceptually:

```go
type Measurement struct {
    Source       SourceType
    Metric       MetricType
    Subject      SubjectType
    Stream       StreamType
    Symbol       string
    Side         Side
    At           time.Time
    ObservedFrom time.Time
    Horizon      time.Duration
    Unit         Unit
    Raw          float64
    Normalized   float64
    Maturity     float64
    Uncertainty  Uncertainty
    Validity     Validity
    Scale        ScaleReference
}
```

The concrete implementation must use existing project types where suitable and
must not copy this illustrative structure mechanically.

### Measurement requirements

- `Raw` retains dimensional and economic meaning.
- `Normalized` supports symbol-local and cross-sectional comparison.
- A missing normalized value is explicit; it is not zero.
- `Horizon` identifies the interval represented by the estimate.
- `Maturity` reports evidence sufficiency, not directional confidence.
- `Uncertainty` describes estimator uncertainty where it can be calculated.
- `Validity` carries failed assumptions, stale inputs, and synchronization
  failures.
- `Scale` identifies the adaptive baseline or normalization epoch.
- Measurements are immutable after emission.

### Signal responsibility

Signals may perform substantial mathematical conditioning, including:

- Adaptive normalization.
- Point-process estimation.
- Order-flow calculation.
- Cross-sectional residualization.
- Spectral decomposition.
- Physical field estimation.

Signals must not emit:

- Entry or exit baselines.
- Portfolio utility.
- Trade direction solely because a metric is signed.
- Contextual market categories.
- Generic classifier probabilities over named narratives.

## Logic composition

Logic receives the Thesis and constructs time-aligned evidence epochs for one
symbol. An epoch must preserve the exact measurements that contributed to it.

Conceptually:

```go
type LogicEpoch struct {
    At            time.Time
    Measurements  []Measurement
    Graph         []Graph
    Categories    []Category
    Hypotheses    []Hypothesis
    Readiness     Readiness
}
```

### Edge Relationship types

The Graph should be able to represent:

- Supports.
- Contradicts.
- Conditions.
- Leads.
- Lags.
- Redundant with.
- Independent of.
- Stale relative to.
- Incomparable with.

These relationships must retain evidence references and temporal context.

### Category formation

Categories are assigned from combinations of measurements. They are not
declared by individual signals.

A category state must retain:

- Supporting evidence.
- Opposing evidence.
- Missing required evidence.
- Measurement maturity.
- Measurement uncertainty.
- Freshness.
- Redundancy adjustment.
- Historical calibration.

Categories can include market contexts such as accumulation, ignition,
supported continuation, fragile continuation, distribution, exhaustion, and
liquidity collapse. Their exact taxonomy must follow validated compositions,
not this initial list.

### Manifold

The manifold composes compatible numerical measurements or physical market
state. It must preserve measurement semantics, time, direction, and uncertainty.
It must not use source or category ordinals as physical coordinates.

### Resonance

Resonance measures persistent agreement, persistent contradiction, phase
relationships, recurring sequences, novelty, and prediction error. Agreement
does not imply a buy; a collapse can also be highly resonant.

### Causal reasoning

Causal reasoning compares explanations of the current state and records what
future observation would discriminate between them. It does not choose the
portfolio action.

## Thesis ownership and structure

The Thesis is the complete in-memory causal record for one tick lifecycle.
It is deliberately kept relatively simple and flat, to prevent an explosion
of complex types.

```go
/*
Thesis is essentially the "state" of a tick. It travels across the
entire lifecycle of a tick, picking up all data along the way.
*/
type Thesis struct {
	uiHub        chan<- []byte
	Signals      *sync.Map
	CrossSection *CrossSection
	Measurements []Measurement
}
```

## Entry, continuation, exit, and reversal theses

A single bullish entry interpretation is insufficient for managing a position.
Each evaluation epoch must construct separate propositions for entry,
continuation, exit, and reversal.

These are related sub-theses within the parent Thesis, not separate disconnected
objects.

### Entry thesis

The entry thesis answers:

> Why should opening this position now have positive executable utility?

It contains:

- Expected outcome distribution.
- Forecast horizon.
- Required supporting evidence.
- Contradicting evidence.
- Entry price and liquidity assumptions.
- Expected fees, spread, impact, and adverse selection.
- Proposed exposure.
- Conditions that invalidate entry before execution.
- Conditions expected to support continuation after entry.

### Continuation thesis

The continuation thesis answers:

> Why is holding the current exposure still preferable to reducing or exiting?

It must be recomputed from current evidence. It must not inherit its truth from
the fact that the entry thesis once passed.

It contains:

- Remaining expected executable return.
- Updated outcome horizon.
- Current risk distribution.
- Current exit liquidity.
- Evidence that the original mechanism persists.
- New supporting and opposing evidence.
- Opportunity cost of occupied capital and slot capacity.
- Utility of holding versus feasible alternatives.

### Exit thesis

The exit thesis answers:

> Why is reducing or closing the position now preferable to continuing to hold?

Exit evidence may include:

- Continuation utility crossing below exit utility.
- Original mechanism invalidation.
- Distribution or exhaustion evidence.
- Deteriorating executable liquidity.
- Adverse-selection increase.
- Forecast expiry.
- Portfolio risk or superior rotation opportunity.
- Risk-limit intervention.

An exit thesis records whether the exit is:

- Thesis invalidation.
- Profit realization.
- Risk containment.
- Liquidity deterioration.
- Rotation.
- Operational necessity.

These causes must not be collapsed into one sell score.

### Reversal thesis

The reversal thesis answers:

> Has the evidence moved beyond weakening the original thesis and begun to
> support the opposite market hypothesis?

Reversal is not merely a negative entry score. It requires positive evidence
for the opposing mechanism.

The reversal assessment should separate:

1. Original thesis weakening.
2. Original thesis invalidation.
3. Opposing thesis formation.
4. Opposing thesis maturity.
5. Utility of exiting only.
6. Utility of reversing exposure, if the account and strategy support it.

In a spot-only long system, a reversal thesis usually supports urgent exit or
continued observation. It must not fabricate a short action that the broker
cannot execute.

### Exit evaluation is continuous

Every new evidence epoch for a held position must compare at least:

```text
hold
reduce
exit
rotate
```

where supported, it may also compare increase or reverse. The chosen action is
the feasible alternative with the greatest expected utility, including the
utility of doing nothing.

### Hysteresis without magic thresholds

Entry and exit must not oscillate because of small estimator changes. Stability
must come from:

- Forecast uncertainty.
- Execution costs.
- State-transition probability.
- Evidence persistence.
- Decision latency.
- Explicitly measured switching cost.

Do not add an arbitrary entry/exit score gap and call it hysteresis.

## Forecast and strategy utility

Forecasts describe outcome distributions. Strategy evaluates the utility of
actions conditional on those forecasts, the current portfolio, and account
state.

Utility belongs to an action alternative:

```text
U(action | Thesis, Portfolio, Account, Market)
```

Entry utility must include expected executable return, fees, spread, impact,
adverse selection, capital usage, portfolio risk, and exit feasibility.

Hold utility must use remaining expected return rather than the original entry
forecast.

Exit utility must include realized execution cost and avoided downside.

Rotation utility must compare the complete portfolio transition:

```text
U(exit incumbent) + U(enter candidate) - switching costs - U(hold incumbent)
```

Reserved opportunity capacity must be an explicit allocation class selected by
strategy. It must not be inferred only because normal slots are full.

## Decision journal

Decisions are stored directly on the Thesis. No Thesis identifier is required
for the in-memory relationship.

A lifecycle may contain entry, hold, reduce, protection, exit, and rotation
decisions, so the Thesis owns a chronological decision journal rather than one
decision field.

A decision must record:

- Action.
- Symbol.
- Evaluation time.
- Utility of the selected action.
- Utilities of feasible alternatives.
- Allocation class.
- Proposed size and price constraints.
- Validity duration or invalidation conditions.
- Evidence and forecast provenance.
- Portfolio context used in evaluation.
- Reason for selection.

The Decision must not point back to the Thesis. The Thesis owns the Decision.

`strategy.Intent` remains the command passed to broker and carries the Thesis
directly while the lifecycle is in-process.

## Broker and trade journal

Broker receives the selected Intent, validates current feasibility, and records
the result on the originating Thesis.

The live mutable `broker.Position` must not be placed directly on the Thesis.
It owns connections, locks, order identity, and mutable folding state. Instead,
broker appends immutable domain observations.

### Trade journal contents

- Intent submission.
- Broker acceptance or rejection.
- Order acknowledgements.
- Order state transitions.
- Partial and complete fills.
- Fill price and quantity.
- Fees.
- Cancellations.
- Entry and exit slippage.
- Position snapshots.
- Protection changes.
- Execution and reconciliation errors.
- Final realized outcome.

### Position association

When an Intent opens a position:

1. Desk associates the new Position with the Intent's Thesis.
2. Position lifecycle updates append immutable observations to that Thesis.
3. Subsequent strategy decisions append to the same Thesis.
4. Closing execution finalizes the Trade journal.
5. The completed lifecycle is handed to PostMortem.
6. Only then may the live closed Position be removed.

This corrects the current behavior where closed positions can be deleted before
their complete history is transferred to a durable in-memory case record.

## Market observations during a trade

The Thesis records the authoritative merged ticker observations used to mark
and manage its position. It should also retain any book, trade, or L3 state used
by a subsequent hold or exit decision through the corresponding measurement
and reasoning epochs.

Do not indiscriminately duplicate raw transport frames on the Thesis. Raw feed
capture belongs to deterministic replay. The Thesis stores the authoritative
market observations and derived evidence relevant to its lifecycle.

Initially retain:

- Every position mark that changes relevant state.
- Every market observation used by a decision.
- Every observation crossing an exit or risk condition.
- Exact timestamps.

Do not apply arbitrary time downsampling. Measure memory use first and design
lossless or decision-preserving compression if required.

Market observations permit calculation of:

- Maximum favorable excursion.
- Maximum adverse excursion.
- Time to peak.
- Time underwater.
- Spread and executable-price path.
- Profit surrendered from peak.
- Forecast-horizon suitability.
- Evidence timing relative to entry and exit.

## Lifecycle states

Use explicit state transitions. The precise names can follow repository
conventions, but the lifecycle must distinguish:

```text
observing
shaped
entry_selected
entry_submitted
partially_entered
entered
managing
exit_selected
exit_submitted
partially_exited
closed
post_exit_observation
postmortem_ready
evaluated
expired
rejected
invalid
```

Invalid transitions return descriptive errors. Do not silently coerce state.

## PostMortem

The current `strategy.PostMortem` is a shell. Its completed implementation must
evaluate distinct layers rather than diffing arbitrary snapshots.

### Eligibility

A traded Thesis is PostMortem-ready only when:

1. An entry decision exists.
2. Broker accepted the entry request.
3. At least one entry quantity filled.
4. The Position remained associated with the same Thesis.
5. Position quantity returned to zero.
6. Entry and exit executions were reconciled.
7. Fees and realized outcomes are available.
8. The required post-exit observation tail completed.

The post-exit horizon is derived from the Thesis forecast horizons and model
memory. It is not one universal duration.

### Layered analysis

PostMortem evaluates:

1. Measurement integrity and calibration.
2. Logic composition and treatment of conflict.
3. Category and hypothesis support.
4. Forecast calibration and horizon suitability.
5. Entry decision utility versus alternatives.
6. Continuation and hold decision quality.
7. Exit and reversal timing.
8. Execution quality.
9. Position management.
10. Portfolio and reserved-slot opportunity cost.

### Outcome decomposition

The report must distinguish:

```text
forecast opportunity
decision timing
execution quality
position management
exit quality
unavoidable realization variance
```

This prevents changes to a correct signal when the actual defect was execution,
or changes to execution when the forecast itself was uncalibrated.

### Counterfactuals

Only bounded, executable counterfactuals may be evaluated, including:

- No entry.
- Entry at the next recorded executable price.
- Exit at first recorded invalidation.
- Hold until forecast expiry.
- Hold until utility crossed zero.
- Rotation versus holding the incumbent.

PostMortem must not invent fills where recorded liquidity was insufficient.

### Findings

PostMortem emits typed findings containing:

- Responsible component.
- Observable condition.
- Evidence.
- Estimated effect.
- Uncertainty.
- Proposed adjustment.
- Required validation.
- Current and candidate model versions.

One PostMortem does not directly mutate a live model.

## Constrained improvement loop

Candidate improvement follows:

```text
individual findings
    -> aggregate comparable completed Theses
    -> estimate systematic error
    -> produce candidate adjustment
    -> deterministic replay
    -> chronological walk-forward validation
    -> compare calibration, utility, and stability
    -> accept or reject a versioned update
```

Safe candidate adjustments include:

- Calibration correction.
- Measurement validity conditions.
- Uncertainty correction.
- Redundancy treatment.
- Conflict interpretation.
- Forecast model selection.
- Entry, continuation, or exit policy changes.
- Execution policy changes.

The loop must not perform immediate single-trade parameter mutation, loss
chasing, or winner reinforcement.

## Implementation phases

### Phase 1: Shared typed contracts

- Define Measurement identity, value, units, horizon, maturity, uncertainty,
  validity, and scale contracts.
- Define immutable evidence references.
- Define Thesis journals and lifecycle state.
- Move the shared Thesis contract out of `strategy` so `logic` no longer depends
  on a strategy implementation package.
- Preserve compilation through explicit migration adapters only while active
  consumers are moved.

Exit criteria:

- A measurement can be interpreted without knowing its source implementation.
- Thesis history is typed and chronological.
- Invalid lifecycle transitions are tested.

### Phase 2: Numerical signal migration

Migrate signals from category outputs to measurements, beginning with the
signals currently under review:

1. Hawkes.
2. Pumpdump.
3. Toxicity.
4. Depthflow.
5. CVD.
6. Liquidity.
7. Remaining signals.

For every signal:

- Specify every metric mathematically.
- Preserve raw and normalized values.
- Add time and horizon semantics.
- Report readiness and invalidity explicitly.
- Remove entry/exit baseline generation.
- Add calculation tests and benchmarks.

Exit criteria:

- No migrated signal assigns a contextual category.
- Measurements retain sufficient information for composition and replay.

### Phase 3: Logic composition and categories

- Align measurements by event time and horizon.
- Add relationship, conflict, redundancy, and freshness state.
- Form categories from multiple measurements.
- Preserve supporting, opposing, and missing evidence.
- Update manifold, resonance, and causal stages to consume typed measurements.

The first end-to-end composition should be the pump lifecycle because it
requires complementary pumpdump, Hawkes, toxicity, depthflow, liquidity, and
cross-sectional evidence.

Exit criteria:

- Conflict survives composition.
- Categories can be traced to exact measurements.
- Logic emits no portfolio action.

### Phase 4: Forecast journal

- Define explicit forecast targets and horizons.
- Store forecast distributions and calibration state on the Thesis.
- Distinguish entry, continuation, exit, and reversal forecasts.
- Add chronological replay evaluation.

Exit criteria:

- Every strategy input is a forecast or observable portfolio constraint.
- Forecast outcomes can be scored independently of P&L.

### Phase 5: Strategy alternatives and decisions

- Implement action-specific utility.
- Compare do-nothing, enter, hold, reduce, exit, rotate, and reserved-capacity
  alternatives where feasible.
- Append selected Decisions and alternative utilities to the Thesis.
- Remove generic category-graph scoring as the final decision mechanism.

Exit criteria:

- Entry and exit use the same current executable-utility framework.
- A held position is never justified only by its original entry thesis.
- Reversal is distinguished from weakening and invalidation.
- Reserved slots require an explicit strategy allocation class.

### Phase 6: Broker lifecycle attachment

- Carry the Thesis directly on Intent.
- Associate Position with its originating Thesis.
- Append immutable order, execution, position, and market observations.
- Finalize the Trade journal before deleting a closed Position.
- Record broker rejection without changing the Decision.

Exit criteria:

- A completed Thesis reconstructs the intended and realized trade.
- Partial fills, fees, and position marks are represented.
- No closed lifecycle is lost when Desk removes runtime state.

### Phase 7: Exit and reversal management

- Recompute continuation, exit, and reversal theses on every relevant evidence
  epoch.
- Compare action utilities with current executable prices and liquidity.
- Record why an exit was selected.
- Add pump-specific multi-leg continuation and collapse fixtures alongside
  ordinary structured-trade fixtures.

Exit criteria:

- Exit behavior responds to thesis invalidation, opposing-thesis formation,
  liquidity deterioration, forecast expiry, and rotation utility.
- Tests prove that weakening, invalidation, and reversal cause distinct state.

### Phase 8: PostMortem

- Implement eligibility and post-exit observation completion.
- Produce layered analysis and outcome decomposition.
- Add bounded counterfactual evaluation.
- Emit typed findings and candidate adjustments.
- Aggregate findings without mutating the running system.

Exit criteria:

- A finding identifies the responsible layer.
- A losing outcome does not automatically condemn its decision.
- A profitable outcome does not automatically validate its decision.

### Phase 9: Validated improvement loop

- Build deterministic completed-Thesis replay.
- Add chronological walk-forward validation.
- Version candidate calibrations and policy changes.
- Require demonstrated improvement without unacceptable stability regression.

Exit criteria:

- No live change is applied from one PostMortem.
- Accepted adjustments carry evidence, validation output, and version history.

### Phase 10: Remove legacy semantics

- Remove signal-owned categories.
- Remove signal-owned entry and exit baselines.
- Remove string-to-`any` Thesis evidence.
- Remove generic reserved-slot admission based only on an edge multiplier.
- Remove decision logic from broker.
- Remove compatibility adapters after all consumers migrate.

No legacy fallback remains in the runtime path.

## Verification requirements

### Unit tests

- Measurement dimensional and normalization integrity.
- Thesis lifecycle transition validity.
- Evidence immutability and provenance.
- Time and horizon alignment.
- Conflict classification.
- Entry, continuation, exit, and reversal distinction.
- Action-specific utility.
- Position-to-Thesis attachment.
- Partial-fill and closure lifecycle.
- PostMortem eligibility.
- Layer-specific findings.

### Scenario tests

- Sound entry and sound exit.
- Sound entry with adverse realization.
- Unsound entry with accidental profit.
- Original thesis weakens and recovers.
- Original thesis invalidates without reversal.
- Mature opposing thesis produces exit.
- Rotation loses utility after execution costs.
- Pump ignites, pulls back, re-ignites, distributes, and collapses.
- Broker rejects a valid strategy decision.
- Execution defect harms a correct forecast.
- Conflict correctly reduces confidence without destroying evidence.

### Replay tests

- Identical inputs produce identical Thesis journals and Decisions.
- Legal batching does not change chronological meaning.
- No future evidence enters an earlier forecast or decision.
- Counterfactuals use only recorded executable market state.

### Benchmarks

Benchmark all migrated signal calculations, logic composition, Thesis journal
growth, utility comparison, broker observation append, and PostMortem analysis
using realistic symbol counts and lifecycle lengths.

## Completion gates

The architecture is complete only when:

1. Signals emit measurements without market narratives or action semantics.
2. Logic owns categories and preserves conflict and provenance.
3. Thesis carries the entire evidence, decision, execution, and outcome history.
4. Entry, continuation, exit, and reversal are separately represented.
5. Strategy compares action-specific utility including no action.
6. Broker executes without silently changing strategy intent.
7. Closed Position history reaches the Thesis before runtime deletion.
8. PostMortem attributes errors to the correct system layer.
9. Candidate improvements require aggregate replay and walk-forward validation.
10. No hidden fallback retains the previous category-score decision path.


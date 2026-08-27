# Opportunity-First Decision Architecture

> **Status:** Architectural specification  
> **Scope:** Live opportunity detection, causal valuation, MCTS arbitration, and decision semantics  
> **Repository baseline:** `TheApeMachine/symm`, reviewed at commit `d99579bce658b39aafe1a63604035dc467fbc19e`

---

## 1. Purpose

SYMM exists to:

1. detect economically interesting opportunities,
2. identify the highest perceived-value opportunities available at a given moment,
3. allocate capital among those opportunities and existing positions,
4. execute and manage positions,
5. learn from hindsight whether detection, valuation, selection, execution, or management was wrong.

The system is **not** required to predict an exact future price path.

Price is one observable outcome of market structure. It is not the sole definition of an opportunity and it is not the only valid target of estimation.

A useful opportunity may exist because of structural asymmetry before price visibly moves: liquidity withdrawal, Hawkes excitation, aggressive-flow buildup, hidden absorption, leverage buildup, cross-asset lag, book thinning, compression, toxicity, basis stress, phase-transition behavior, or another empirically supported precursor.

The live architecture MUST preserve a hard distinction between:

```text
opportunity detected
causal valuation available
economic utility estimated
action selected
```

These are separate states.

---

## 2. Core Principle

> **Causal evaluation unavailable does not mean no opportunity exists.**

And:

> **No opportunity does not mean zero utility.**

And:

> **Zero utility does not mean evaluation was unavailable.**

Those states MUST NOT collapse into one representation.

A symbol may have a real precursor while a causal transition model is undefined, rank deficient, under-supported, temporarily unavailable, or otherwise not identified.

That is a statement about one model query. It is not a statement that the market contains no opportunity.

---

## 3. Current Failure Mode

The current planner path treats causal identification as a global admission gate.

Conceptually, current behavior is:

```text
Measurements
    ↓
Reasoner
    ↓
build full per-symbol causal state
    ↓
require whole state identified
    ↓
if any required/current transition is unavailable:
    ActionNothing
    Utility = default zero
    MCTS does not run
```

This produces displays such as:

```text
LSETH/USD
u=0.0000 nothing
planner: causal evaluation unavailable: undefined

PEPE/USD
u=0.0000 nothing
planner: causal evaluation unavailable: insufficient_rank

GTC/USD
u=0.0000 nothing
planner: causal evaluation unavailable: insufficient_support
```

These rows currently conflate at least three distinct facts:

```text
1. no economic evaluation occurred
2. causal estimation was unavailable
3. Utility was never populated and therefore remained Go's zero value
```

The visible `u=0.0000` is therefore not evidence that utility was estimated to be zero.

This is semantically incorrect.

---

## 4. Required State Semantics

The decision path MUST distinguish these states explicitly.

### 4.1 Opportunity state

A symbol/archetype may be:

```text
none
forming
armed
active
invalidated
resolved
```

The exact lifecycle may vary by archetype, but opportunity existence is independent of causal-model readiness.

### 4.2 Valuation state

An opportunity valuation may be:

```text
not_attempted
available
unavailable
degraded
```

If unavailable, the reason MUST be explicit.

Examples:

```text
undefined
insufficient_support
insufficient_rank
not_identifiable
unsupported_treatment
missing_execution_market
```

### 4.3 Utility state

Utility MUST have explicit availability.

Conceptually:

```go
type EconomicValue struct {
    Value     float64
    Available bool
}
```

or an equivalent typed representation.

The wire/UI MUST render:

```text
u=—
```

when no economic objective was evaluated.

It MUST render:

```text
u=0.0000
```

only when an actual economic evaluation produced zero.

---

## 5. Opportunity Detection Is Its Own First-Class Stage

SYMM MUST have a first-class live representation of an opportunity.

The system already contains multiple independent forms of evidence:

- raw and derived Measurements,
- Category,
- Cognition,
- Manifold,
- Relation / Influence,
- causal structure,
- Resonance,
- passage statistics,
- portfolio state,
- execution-market state.

These outputs MUST NOT be forced through one universal model before they are allowed to represent an opportunity.

A useful conceptual pipeline is:

```text
Market Events
    ↓
Signals / Measurements
    ↓
┌──────────────┬──────────────┬──────────────┬──────────────┐
│   Category   │   Manifold   │ Relation /   │  Resonance   │
│              │              │   Causal     │              │
└──────┬───────┴──────┬───────┴──────┬───────┴──────┬───────┘
       │              │              │              │
       └──────────────┴───────┬──────┴──────────────┘
                              ↓
                     Opportunity State
                              ↓
                    Opportunity Valuation
                              ↓
                     Portfolio Arbitration
                              ↓
                            MCTS
                              ↓
                    Allocation / Execution
                              ↓
                          Hindsight
```

No subsystem owns a Measurement merely because it consumes it.

For example, Hawkes may simultaneously act as a forcing mechanism for Manifold, causal evidence, precursor evidence, category evidence, and telemetry. That is valid fan-out.

---

## 6. Opportunity Is a Typed Hypothesis, Not a Generic Score

SYMM MUST NOT create a universal scalar such as:

```text
opportunity_score = 0.87
bullishness = 0.73
setup_quality = 0.91
```

as the primary semantic contract.

A scalar may exist as a derived economic quantity only when its units and meaning are explicit.

A first-class opportunity should instead carry typed, economically meaningful state.

Conceptually:

```go
type OpportunityState struct {
    Symbol     string
    Archetype  OpportunityType
    Phase      OpportunityPhase
    Direction  Direction
    At         time.Time

    // Optional, model-defined quantities.
    TransitionProbability OptionalFloat
    FavorableExcursion    DistributionRef
    AdverseExcursion      DistributionRef
    ResolutionTime        DistributionRef

    // Execution context.
    SpreadFraction OptionalFloat
    ImpactEstimate OptionalFloat
    ExecutableDepth OptionalFloat

    // Provenance / availability.
    EvidenceMask      EvidenceMask
    ValuationStatus   ValuationStatus
    Identification    IdentificationStatus
}
```

The exact struct is implementation-defined. The semantics are not.

---

## 7. Opportunity Archetypes

Opportunity archetypes represent classes of economic asymmetry.

Examples include, but are not limited to:

- vertical ignition precursor,
- slow pump,
- fast pump,
- liquidity vacuum,
- short squeeze,
- leveraged ignition,
- hidden accumulation / absorption,
- algorithmic coattail,
- breakout from compression,
- mean-reverting dislocation,
- cross-asset lag exploitation,
- liquidation cascade,
- toxic-liquidity retreat,
- exhaustion / reversal,
- manipulation precursor,
- transient scalp.

Archetypes MUST be evidence-driven.

They MUST NOT become a giant hand-authored set of hard-coded buy/sell rules.

Existing semantic outputs such as Category are evidence about market state. They are not automatically trading actions.

---

## 8. Precursor Principle

Visible price movement may occur late in the causal chain.

For an ignition-like event:

```text
precursor
    ↓
structural tension
    ↓
liquidity / flow transition
    ↓
visible ignition
    ↓
settlement
```

The ideal entry may occur before the visible ignition.

Therefore:

> **A system that only recognizes an opportunity after the price target becomes predictable is structurally late.**

SYMM SHOULD prefer evidence about a transition before settlement when that evidence is empirically justified.

Example lifecycle:

```text
DORMANT
   ↓
FORMING
   ↓
ARMED
   ↓
IGNITION
   ↓
MANAGE
   ↓
RESOLVED

or

FORMING
   ↓
INVALIDATED
```

The visible `VerticalIgnition` state may be confirmation of a transition whose economically interesting precursor began earlier.

---

## 9. Causal Models Are Query-Local

This is a non-negotiable requirement.

> **The causal model MUST answer the current economic query using the smallest defensible dependency closure required for that query.**

It MUST NOT require the entire observed market ontology to have identified forward transitions before a candidate may be economically evaluated.

Incorrect:

```text
30 coordinates exist
    ↓
require 30 identified transition models
    ↓
one unrelated coordinate has insufficient_support
    ↓
entire symbol rejected
```

Required:

```text
30 coordinates exist
    ↓
economic query selects outcome / candidate
    ↓
find active dependency closure
    ↓
require only transitions necessary to evolve that closure
    ↓
evaluate if that closure is identified
```

For example:

```text
Outcome
  ├── CVD flow
  │     └── Hawkes excitation
  ├── ask retreat
  │     └── book imbalance
  └── lead/lag
```

If this is the required active closure, then an unrelated unavailable sentiment transition MUST NOT veto the query.

---

## 10. Required Causal Closure

For each economic query:

1. identify the requested outcome,
2. identify the active fitted parents of that outcome,
3. recursively walk active parents needed over the rollout horizon,
4. include required self-lag transitions,
5. include only transitions whose values are required to evolve that state,
6. validate identification for that closure,
7. run the economic evaluation if the closure is valid.

The closure is dynamic and query-local.

It is NOT a global feature-selection pass.

It MUST NOT delete observational data.

All valid Measurements remain available in resident observational state.

---

## 11. Strict Statistical Semantics Remain

This specification does **not** authorize fake estimates.

If a transition in the required causal closure is genuinely unavailable, the query is unavailable.

SYMM MUST NOT silently substitute:

- correlation,
- persistence,
- zero,
- an old coefficient,
- schema-declared lag as if it were measured evidence,
- ridge regression,
- dropped predictors,
- or any other fabricated answer,

unless that behavior is explicitly part of the declared model.

The following statuses remain meaningful.

### `undefined`

The mathematical query cannot currently be formed.

Examples:

- required coordinate absent,
- no target history,
- required lagged state absent.

### `insufficient_support`

There are too few aligned observations relative to model dimensionality.

### `insufficient_rank`

There are enough rows, but the design matrix is not full rank.

This is a mathematical property, not a weak-confidence synonym.

### `not_identifiable`

The requested causal effect is not identifiable under the declared model/schema.

These statuses MUST survive all the way to diagnostics.

---

## 12. Identification Diagnostics Must Preserve Provenance

A naked status such as:

```text
insufficient_rank
```

is not enough.

The live system MUST preserve enough information to identify exactly which transition blocked the query and why.

At minimum:

```text
coordinate
status
observation count
aligned row count
parameter count
numerical rank
self lag
active parents
excluded parents
effective support
fit timestamp
model/schema version
```

Example:

```text
PEPE/USD

opportunity:
    vertical_ignition_precursor / forming

valuation:
    unavailable

blocking transition:
    hawkes/background_rate

status:
    insufficient_rank

rows:
    183

parameters:
    2

rank:
    1

reason:
    self-lag predictor has no independent variation
```

This is actionable.

`planner: causal evaluation unavailable: insufficient_rank` is not.

---

## 13. Causal Unavailability Is Evidence, Not Opportunity Erasure

A detected candidate with unavailable causal valuation remains a candidate.

Example:

```text
Opportunity:
    detected

Archetype:
    vertical_ignition_precursor

Causal valuation:
    unavailable / insufficient_support

Economic action:
    not currently justified by causal valuation
```

This is different from:

```text
Opportunity:
    none
```

Hindsight MUST be able to distinguish the two.

This distinction is essential for learning whether SYMM failed to detect, detected but could not value, detected and valued but ranked incorrectly, or selected correctly but executed poorly.

---

## 14. MCTS Role

MCTS is an intervention optimizer.

It is not the opportunity detector.

Its job is to answer questions such as:

```text
enter now?
wait?
exit existing position?
hold?
scale, if executable?
redeploy capital?
```

given:

- a candidate opportunity,
- an economic model,
- current portfolio state,
- execution costs,
- uncertainty,
- and currently feasible actions.

MCTS MAY use price-return evolution where useful.

It MUST NOT require exact price prediction as the philosophical basis of the opportunity.

Useful outcomes may include:

- net wealth change,
- probability of favorable boundary before invalidation,
- adverse excursion,
- drawdown,
- execution cost,
- time to resolution,
- capital occupancy,
- opportunity cost.

---

## 15. First-Passage Questions Are First-Class

For many opportunities, the relevant question is closer to:

```text
Will profit be reached before invalidation?
```

than:

```text
What exact price will occur at t+N?
```

The architecture SHOULD support opportunity-conditioned first-passage evidence:

- favorable excursion distribution,
- adverse excursion distribution,
- probability profit boundary occurs first,
- time to boundary,
- time to invalidation,
- survivor adverse excursion.

Existing passage learning SHOULD be conditioned by the actual opportunity archetype whenever possible.

---

## 16. Portfolio Arbitration

SYMM's top-level objective is economic.

A candidate MUST compete against:

- other candidates,
- available cash,
- current positions,
- the expected remaining value of current positions,
- transaction friction,
- capital occupancy,
- execution feasibility.

A current position is a use of capital.

Holding a dead position has opportunity cost.

The arbiter SHOULD eventually be able to compare:

```text
keep XYZ
```

against:

```text
exit XYZ
free capital
enter HMAID
```

when both actions are executable and economically modelled.

---

## 17. Ranking Semantics

Do not rank opportunities by arbitrary confidence alone.

Prefer economically interpretable comparisons.

Candidate comparison may consider:

```text
is executable?
is valuation available?
is conservative net outcome positive?
downside / invalidation distribution
expected favorable excursion
time to resolution
capital required
transaction friction
current portfolio displacement cost
model uncertainty
```

Where distributions are not sufficiently calibrated, prefer ordered decision rules over fake precision.

---

## 18. Streaming Invariant

This architecture remains a stream.

> **Events move. State stays.**

Every subscriber gets one chance to consume an event, mutate bounded resident state, and emit derived output.

The opportunity architecture MUST NOT introduce:

- global snapshots,
- world-state clones,
- `[]Work` accumulation,
- unbounded channels,
- deferred per-symbol backlogs,
- repeated reconstruction of the whole universe,
- per-tick materialization of all opportunities,
- full-world planner batches.

Hot-path APIs SHOULD expose resident lookup, direct iteration, incremental update, and fixed/bounded state.

They SHOULD NOT return convenience snapshots of resident state.

---

## 19. Planner Must Not Rebuild the World

The planner MUST move away from the pattern:

```text
retain latest state for all symbols
    ↓
drain all pending states
    ↓
build slice
    ↓
evaluate every symbol
```

Opportunity computation SHOULD follow opportunity density, not universe size.

Required direction:

```text
one candidate changes
    ↓
update one resident opportunity state
    ↓
revalue only affected arbitration state
    ↓
run expensive search only when justified
```

Most symbols should be cheap when nothing interesting is happening.

---

## 20. No New Global Veto

No single subsystem may become a universal readiness veto unless the action truly cannot be executed without it.

Allowed operational vetoes include things such as:

- no executable quote,
- no fee surface,
- no cash state,
- broker unavailable,
- action impossible.

Analytical subsystems are evidence providers.

Examples:

```text
Category unavailable
Manifold unavailable
Cognition unavailable
Causal unavailable
Resonance unavailable
```

do not automatically mean:

```text
opportunity = none
```

Each opportunity archetype defines which evidence is required, optional, or unavailable.

---

## 21. Metrics Have Multiple Legitimate Consumers

A metric is not "owned" by Graph, MCTS, Category, or Manifold.

A single Measurement may legitimately feed multiple downstream consumers.

Example:

```text
Hawkes measurement
    ├── Manifold forcing
    ├── Category
    ├── Relation / causal model
    ├── Opportunity evidence
    └── telemetry
```

The Measurement is computed once and streamed.

Consumers do not duplicate its computation.

---

## 22. Hindsight Must Separate Failure Classes

Hindsight MUST distinguish at least:

### Detection regret

Did SYMM fail to recognize the opportunity early enough?

### Valuation regret

Was the opportunity detected but economically unvalued or misvalued?

### Selection regret

Was the candidate valued correctly but ranked below a worse use of capital?

### Execution regret

Was the right opportunity selected but entered, sized, or routed poorly?

### Management regret

Was the position entered well but exited, stopped, or held poorly?

Useful hindsight fields include:

```text
precursor first-seen time
candidate armed time
visible ignition time
entry time
exit time
lead time
price movement before detection
price movement before entry
MFE
MAE
time to favorable boundary
time to invalidation
realized capture
capital occupied
causal availability
blocking causal transition
selection competitor
```

Perfect-execution price legs remain useful as an upper bound, but they are not the complete learning target.

---

## 23. Hindsight Must Learn From "Detected But Unvalued"

A missed opportunity MUST NOT be classified only as:

```text
planner said nothing
```

If the opportunity subsystem detected a candidate but causal valuation failed, Hindsight should report:

```text
detection: successful
valuation: unavailable
blocking model: ...
realized move afterward: ...
```

This allows the system to learn whether more history was required, the causal closure was unnecessarily broad, a particular model repeatedly fails on exactly the opportunities that matter, another valuation model should be added, or the candidate was correctly left untouched.

---

## 24. Performance Requirements

This change MUST NOT reintroduce the performance failures already observed.

Non-negotiable:

- no global data ring replacement,
- keep existing LMAX/go-disruptor transport,
- no custom general-purpose ring,
- no hidden queue after LMAX,
- no snapshots,
- no clones,
- no unbounded accumulation,
- no per-tick full-universe reconstruction,
- no rendered string identity in computational hot paths,
- no speculative large state allocation per candidate,
- no O(N²) retained state unless explicitly justified and bounded,
- no extra goroutine as a substitute for reducing work.

Total resident state must scale with:

```text
symbols
+
admitted opportunities
+
admitted causal edges
+
bounded model state
```

not blindly with:

```text
symbols² × history × model dimensionality
```

---

## 25. No Hidden Fallbacks

When valuation is unavailable:

```text
unavailable
```

must remain unavailable.

Do not turn it into:

```text
Wait wins
utility = 0
confidence = 0
no opportunity
```

unless those are the actual results of an explicit evaluation.

Likewise, do not invent a heuristic entry solely because causal valuation is unavailable.

Detection and action authority remain distinct.

---

## 26. UI / Telemetry Requirements

The UI must expose semantic state honestly.

Bad:

```text
u=0.0000 nothing
planner: causal evaluation unavailable: insufficient_rank
```

Required:

```text
opportunity: vertical_ignition_precursor / forming
valuation: unavailable
utility: —
action: nothing
causal: insufficient_rank
blocking transition: ...
```

When MCTS actually evaluates and chooses Wait with expected utility zero:

```text
valuation: available
utility: 0.0000
action: wait
```

These must be visually and structurally distinguishable.

---

## 27. Required Decision Semantics

At minimum, a live decision should make these concepts distinguishable:

```text
OpportunityPresent
OpportunityType
OpportunityPhase

ValuationAttempted
ValuationAvailable
ValuationStatus

UtilityAvailable
Utility

Action
ActionReason

CausalIdentification
CausalBlockingCoordinate
```

The exact transport schema may differ.

The semantics may not.

---

## 28. Acceptance Criteria

This architecture is not complete until all of the following are true.

### Causal locality

A symbol with one unrelated under-supported coordinate can still be economically evaluated when the active causal closure needed by the query is fully identified.

### Strict blocking

If a transition inside the required closure is under-supported or rank deficient, the query remains unavailable.

No fallback estimate is fabricated.

### Honest utility

An unevaluated decision never renders or serializes as an evaluated zero utility.

### Opportunity independence

A detected opportunity remains visible even if causal valuation is unavailable.

### Provenance

The exact blocking transition and identification diagnostics are available in telemetry/audit.

### Streaming

No new batch/snapshot/global-drain architecture is introduced.

### Sparse expensive work

MCTS/expensive valuation is driven by relevant candidate updates, not automatically by every symbol every tick.

### Hindsight separation

Hindsight can distinguish:

```text
missed detection
detected but unvalued
misvalued
misranked
execution failure
management failure
```

---

## 29. Tests Required

At minimum:

### Query-local causal gating

Construct a symbol with:

- identified outcome closure,
- one unrelated present transition with `insufficient_support`.

Expected:

```text
economic query remains evaluable
```

### Required transition failure

Construct a symbol whose required outcome parent has `insufficient_support`.

Expected:

```text
valuation unavailable
blocking coordinate preserved
```

### Rank-deficient provenance

Construct a constant self-lag predictor.

Expected:

```text
status = insufficient_rank
rows > parameters
rank < parameters
blocking coordinate reported
```

### Undefined target

No observations for required target coordinate.

Expected:

```text
status = undefined
utility unavailable
```

### Utility zero versus unavailable

Case A:

```text
MCTS evaluated
expected economic outcome = 0
```

Expected:

```text
UtilityAvailable = true
Utility = 0
```

Case B:

```text
causal query unavailable
MCTS not run
```

Expected:

```text
UtilityAvailable = false
```

### Opportunity survives valuation failure

Create a detected precursor candidate and force causal valuation unavailable.

Expected:

```text
candidate remains present
no entry fabricated
valuation status retained
```

### No global world veto

Add an unrelated newly observed coordinate with one sample.

Expected:

```text
existing evaluable candidate does not become globally unavailable
unless the new coordinate enters its required active causal closure
```

---

## 30. Non-Goals

This specification does NOT authorize:

- removing statistical identification checks,
- replacing causal models with heuristic scores,
- forcing every metric into every opportunity archetype,
- making Category directly execute trades,
- making Manifold directly execute trades,
- making Hawkes a buy signal,
- treating correlation as causation,
- predicting exact future prices,
- adding arbitrary confidence thresholds,
- increasing queue/ring capacities to hide overload,
- creating cached world snapshots,
- creating a second scheduler,
- replacing LMAX,
- creating one goroutine per symbol,
- creating pairwise permanent state for every possible pair by default.

---

## 31. Architectural Invariants

These statements are intended to be copied into agent instructions and treated as non-negotiable.

> **Opportunity detection and causal valuation are separate concerns.**

> **Causal evaluation unavailable does not mean no opportunity exists.**

> **No evaluation is not zero utility.**

> **Causal identification is query-local, not a whole-world readiness gate.**

> **Only the transitions required by the active economic query may block that query.**

> **Unavailable evidence remains unavailable; never fabricate a fallback.**

> **Metrics are facts in the stream and may have multiple legitimate consumers.**

> **MCTS optimizes interventions; it does not define whether an opportunity exists.**

> **Hindsight must learn whether failure occurred in detection, valuation, selection, execution, or management.**

> **Events move. State stays. No clones, copies, snapshots, hidden accumulations, or world rebuilds on the hot path.**

> **Expensive computation follows opportunity density, not universe size.**

---

## 32. Target End State

The desired live behavior is:

```text
Market event
    ↓
resident signal state updates
    ↓
semantic / physical / relational evidence updates
    ↓
opportunity candidate changes
    ↓
candidate remains visible whether or not every model is ready
    ↓
query-local valuation attempts to price the opportunity economically
    ↓
if required model closure is unavailable:
    retain candidate + explicit valuation failure
    ↓
if valuation is available:
    compare against other candidates and current positions
    ↓
MCTS evaluates feasible interventions
    ↓
allocation / execution
    ↓
position management
    ↓
hindsight attributes success/failure to the correct stage
```

That is the architecture SYMM should converge toward.

The system is not trying to know the future.

It is trying to recognize asymmetric situations early, judge them as honestly as its available evidence permits, deploy capital where the economic case is strongest, and learn exactly why it was right or wrong afterward.

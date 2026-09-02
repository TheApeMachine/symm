# Hindsight System Specification

## 1. Purpose

Hindsight is SYMM's retrospective system-inspection and mathematical-validation
engine.

It exists to answer:

> When the market entered an objectively interesting historical condition, what
> exactly did SYMM know, calculate, retain, infer, and produce at that moment,
> and was that machinery mathematically, semantically, numerically, temporally,
> and causally sane?

Hindsight is NOT:

- a strategy tuner;
- a threshold optimizer;
- a parameter search engine;
- a profit simulator;
- a regret calculator;
- a "what should we have done?" engine;
- a system for finding changes that would have captured historical returns.

Future market data may tell Hindsight where an interesting historical moment
was.

Future market data may never alter what SYMM knew at that moment.

The core purpose is inspection.

---

## 2. Core Laws

### Law 1 — Future Selects, Past Determines

> The future may tell Hindsight where to look.
> It may never change what SYMM knew there.

A future +20% excursion may identify an earlier reference point.

The +20% future return is not an input to the reconstructed SYMM state at that
reference point.

---

### Law 2 — Exact Provenance

> Every derived fact must be traceable to the exact captured external input that
> caused its computation and the exact resident state version that participated.

There must never be uncertainty about which websocket frame produced which
Envelope, Measurement, Perspective, Category, Opportunity, Decision, or other
artifact.

Timestamp proximity is not provenance.

---

### Law 3 — Correctness, Not Profit

> Hindsight judges whether the machinery was sane.
> It never claims that sane machinery would have captured the observed outcome.

A mathematically valid system may select CASH immediately before a +40% market
move.

If all component contracts were satisfied, Hindsight reports no defect.

A profitable live trade may contain broken mathematics.

If a component contract was violated, Hindsight reports the defect regardless
of the trade outcome.

---

## 3. Fundamental Separation

Hindsight operates over three distinct domains:

    Capture
        What external reality SYMM actually received.

    Witness
        What the running SYMM actually produced from that reality.

    Episode
        What became interesting when the market was viewed retrospectively.

These domains MUST remain separate.

Conceptually:

    Raw market / exchange data
              |
              v
          Capture Tape
              |
              v
        Live SYMM execution
              |
              v
          Witness Tape

Later:

        Capture Tape
              |
              +------------------+
              |                  |
              v                  v
       Episode Discovery   Replay Inspection
              |                  |
              +--------+---------+
                       |
                       v
                System Inspection
                       |
                       v
                   Validation

Episode discovery does not feed the trading system.

Hindsight validation does not feed the trading system.

There is no feedback edge.

---

## 4. Capture Identity

Every external input MUST receive a stable identity BEFORE parsing.

The identity MUST NOT depend on:

- a SQLite auto-increment row ID;
- a venue timestamp;
- a venue sequence number;
- the eventual persistence backend;
- the parsed event type.

A logical capture identity should contain at least:

    Run
    CaptureSequence
    Stream
    StreamEpoch
    StreamSequence

Example:

    CaptureRef {
        Run
        Sequence
        Stream
        StreamEpoch
        StreamSequence
    }

Exact Go types are implementation details.

The semantics are not.

---

## 5. Run Identity

Every process capture session belongs to one Run.

A Run MUST identify enough execution context to make its data interpretable.

At minimum:

    RunID
    process start time
    code commit
    build identity
    configuration digest
    schema versions

A Run boundary MUST be explicit across restart.

Capture sequence numbers MUST NOT silently continue across unrelated process
runs unless the identity still makes the run boundary unambiguous.

---

## 6. Capture Sequence

`CaptureSequence` is the monotonically increasing order in which SYMM observed
external inputs during one Run.

It answers:

> What had SYMM observed before this input arrived?

It is assigned locally before parsing.

It is not exchange time.

It is not market-event time.

It is not wall-clock sorting performed after capture.

For causal replay, the capture sequence is the primary ordering of external
observations unless a specific protocol supplies stronger ordering semantics
inside one captured frame.

---

## 7. Stream Identity and Epoch

Each transport stream must be distinguishable.

Examples include:

    spot public
    spot private
    spot Level3 child connection
    futures
    REST response where captured
    other explicit external sources

A reconnect creates a new `StreamEpoch`.

`StreamSequence` is monotonic within one stream epoch.

This allows Hindsight to distinguish:

    frame 81 before disconnect

from:

    frame 81 after reconnect

without relying on timestamps.

---

## 8. Time Semantics

Hindsight MUST preserve different concepts of time separately.

### 8.1 Capture order

    CaptureSequence

Defines local observation order.

---

### 8.2 Receive time

    ReceivedAt

Defines when the process received the external input.

Useful for:

- latency inspection;
- process health;
- transport delay;
- observer availability.

It is not market-event identity.

---

### 8.3 Venue event time

Examples:

    ticker.Timestamp
    trade.Timestamp
    Level3.Timestamp
    futures event time

Defines the time claimed by the venue for the market event.

It may differ from `ReceivedAt`.

---

### 8.4 Venue sequence

Where supplied:

    futures seq
    channel sequence
    other protocol sequence

must be retained independently.

Venue sequence may provide strong ordering semantics within its protocol domain.

It does not replace global local capture identity.

---

## 9. Never Correlate by Timestamp Alone

Hindsight MUST NOT infer:

> these records probably belong together because their timestamps are close.

Network delay, clock skew, batching, snapshots, reconnects, distinct venue
clocks, and local scheduling make this unsafe.

Timestamps are evidence.

Identity establishes provenance.

---

## 10. Raw Frame Capture

The raw external payload is the irreducible replay substrate.

A RawFrame logically contains:

    CaptureRef
    ReceivedAt
    Endpoint / transport
    Kind
    Payload
    PayloadHash

Payload bytes SHOULD remain exactly as received where practical.

The raw tape must not contain reconstructed or normalized equivalents in place
of the actual input.

Derived representations may exist separately.

---

## 11. Capture Happens Before Parse Provenance

The required ingress flow is:

    socket receives bytes
           |
           v
    assign CaptureRef
           |
           +------> persist raw frame
           |
           v
         parse
           |
           v
    produce 0..N Envelopes
           |
           v
        Workspace

The same CaptureRef assigned to the raw bytes MUST be carried into every
Envelope derived from those bytes.

The parser does not invent a new origin.

---

## 12. One Raw Frame May Produce Zero, One, or Many Envelopes

A raw websocket frame may contain:

- one ticker observation;
- multiple trades;
- multiple Level3 symbol updates;
- a snapshot containing many items;
- protocol metadata producing no semantic market Envelope.

Therefore:

    RawFrame -> Envelope

is not necessarily one-to-one.

Every produced Envelope MUST have a deterministic ordinal inside its raw frame.

Conceptually:

    EnvelopeRef {
        CaptureRef
        Ordinal
    }

Example:

    Capture 9123
        -> Envelope 9123:0
        -> Envelope 9123:1
        -> Envelope 9123:2

The ordinal is assigned in deterministic parser order.

---

## 13. Envelope Manifest

Capture storage SHOULD record enough information to prove exactly how a raw
frame entered Workspace.

An EnvelopeManifest logically contains:

    Run
    CaptureSequence
    EnvelopeOrdinal
    Workload
    DomainKind
    Symbol / key where applicable
    VenueTime where applicable
    VenueSequence where applicable

This provides the immutable relationship:

    raw external input
        ->
    exact semantic Workspace ingress

There should be no reconstruction of that relationship after the fact.

---

## 14. Envelope Provenance

Every Workspace Envelope MUST carry its `EnvelopeRef`.

Conceptually:

    Envelope {
        Origin EnvelopeRef
        ...
    }

That identity remains unchanged for the complete ring traversal.

The Envelope may accumulate semantic output.

Its origin does not change.

---

## 15. One Event, One Trip

Hindsight must preserve the Workspace runtime contract:

> One event. One slot. One trip through one workload graph.

The Envelope is the travelling computation.

Hindsight MUST NOT invent intermediate synthetic Envelopes in order to make
provenance easier.

A signal result produced during an Envelope's traversal belongs to that
Envelope's origin.

---

## 16. Artifact Identity

Every durable semantic artifact SHOULD have a stable identity.

Examples:

    Measurement
    Perspective
    Category
    Graph update
    Resonance artifact
    Manifold state observation
    Cognition
    Opportunity
    Valuation
    Planner result
    Decision
    Execution calculation
    Position-risk transition

An ArtifactID must allow Hindsight to distinguish two artifacts even when they
have identical values and timestamps.

---

## 17. Immediate Provenance

Derived artifacts SHOULD reference their immediate causal inputs.

For example:

    RawFrame
       |
    Envelope
       |
    CVD Measurement
       |
    Question-specific Perspective (when configured)
       |
    Valuation

A future Perspective does not need to duplicate the entire history of the CVD
estimator.

It needs to identify the CVD Measurement it consumed.

The provenance graph can then be traversed recursively.

Prefer:

    immediate parent references

over:

    giant repeated transitive provenance blobs.

---

## 18. Resident State Provenance

Stateful processors consume more than the current Envelope.

Examples include:

- estimators;
- future named-question Advisors;
- Category;
- Graph;
- Opportunity;
- planner state;
- risk state.

A current output may therefore depend on facts produced by earlier Envelopes.

Resident state MUST retain enough provenance to identify the origin of the
stateful facts being used.

Example:

    Continuation Perspective (when configured) @ ticker Envelope 9188:0

    used:
        Liquidity Measurement <- 9188:0
        CVD Measurement       <- 9174:2
        DepthFlow Measurement <- 9181:0

Hindsight must be able to display exactly this relationship.

---

## 19. State Versions

Cross-Workload components may be advanced concurrently by different Workloads.

`CaptureSequence` describes receive order.

It does NOT necessarily describe the exact order in which concurrently running
Workloads committed transitions into one shared resident semantic state.

Therefore shared stateful components SHOULD expose a monotonic StateVersion.

Conceptually:

    component
    key / symbol
    state version
    triggering EnvelopeRef

Example:

    future.advisor.continuation / BTC
        version 881
        triggered by Trade Envelope 17823:0

    future.advisor.continuation / BTC
        version 882
        triggered by Ticker Envelope 17827:0

This records what ACTUALLY occurred in the running system.

Hindsight must not guess cross-ring transition order after the fact.

---

## 20. Witnesses

A Witness is evidence of what the live system actually produced at an explicit
Workspace boundary.

Workspace observation groups are the natural witness points.

Example:

    Compute: signals
           |
        barrier
           |
    Witness: after-signals
           |
        barrier
           |
    Compute: category
           |
        barrier
           |
    Witness: after-category

A witness observes completed semantic state.

It does not modify that state.

---

## 21. Witness Boundaries

Useful witness boundaries may include:

    ingress
    after-signals
    after-graph
    after-category
    after-cognition
    after-opportunity
    after-valuation
    after-planner
    after-execution
    after-position-risk

The exact graph depends on the Workload.

Boundary names must describe actual configured causal boundaries.

No manually maintained fictional pipeline may disagree with the Workspace
configuration.

---

## 22. Witness What Changed

Hindsight SHOULD NOT restore the old model of serializing the entire accumulating
Envelope after every stage.

That creates:

- redundant storage;
- repeated large blobs;
- unnecessary allocation;
- unclear ownership;
- excessive write amplification.

Prefer recording the semantic artifacts produced at the boundary.

For example:

    after-signals:
        CVD Measurement
        Hawkes Measurement

    after-category:
        Categories

    after-planner:
        StrategyRound

The raw capture remains the complete replay substrate.

The witness tape records what the running system actually produced.

---

## 23. Witness Record

A logical ArtifactWitness may contain:

    Run
    EnvelopeRef
    Boundary
    ArtifactID
    ArtifactKind
    ProducedAt
    Component
    ComponentStateVersion
    ImmediateParents
    Payload

Not every field is required for every artifact.

The logical relationship is required.

Storage representation is implementation-specific.

---

## 24. Historical Witness vs Replay Inspection

Hindsight MUST distinguish two fundamentally different inspection modes.

### 24.1 Historical Witness

Answers:

> What did the actual running binary produce at that time?

Historical Witness reads stored artifacts.

It does not recompute them.

If the live Hawkes implementation emitted incorrect mathematics, Historical
Witness must show the incorrect result.

That is evidence.

---

### 24.2 Replay Inspection

Answers:

> What does this specified build produce when given the same captured external
> input sequence?

Replay runs the raw capture through the selected build/configuration.

A replay MUST record:

    source Capture Run
    replay Run ID
    code commit
    build identity
    configuration digest
    schema versions

Replay output must never overwrite or masquerade as Historical Witness output.

---

## 25. Replay Is Not Counterfactual Profit

Replay may demonstrate:

> The corrected Hawkes implementation now satisfies its mathematical contract
> over this historical episode.

Replay MUST NOT automatically conclude:

> Therefore this historical move would now have been captured.

Changing an upstream calculation changes downstream state.

Historical market outcome alone cannot prove what a changed complete system
would have done under a future recurrence.

---

## 26. Episode

An Episode is an objectively interesting region of historical market behavior.

Examples:

- upward excursion;
- downward excursion;
- reversal;
- volatility expansion;
- volatility contraction;
- liquidity collapse;
- spread expansion;
- depth migration;
- arrival clustering;
- structural symmetry;
- correlation breakdown;
- cross-symbol propagation;
- actual position entry;
- actual position exit;
- stop event;
- adverse execution;
- reconnect;
- observer startup during an active move.

An Episode describes market or operational reality.

It does not describe what SYMM should have done.

---

## 27. Episode Selection Is Independent

Market Episodes MUST be discoverable without using SYMM's trading outputs as
selection criteria.

Do not select an Episode because:

- Opportunity was high;
- a future Perspective survived;
- Planner almost entered;
- strategy entered;
- strategy failed to enter;
- a threshold nearly passed.

Market Episode discovery should operate on declared external market coordinates.

This avoids selecting evidence that already agrees with the system.

---

## 28. Reference Points

An Episode contains one or more ReferencePoints.

Examples:

    Anchor
    Trough
    Peak
    Reversal
    ExitAnchor
    ShockOnset

A ReferencePoint is retrospective geometry.

It is NOT a recommendation.

For example:

> Anchor is the retrospectively optimal start of the selected +20% price
> excursion.

does not mean:

> SYMM should have bought at Anchor.

---

## 29. Market Coordinate Must Be Declared

Every Episode selector must declare its coordinate.

Examples:

    trade price
    midpoint
    best bid
    best ask
    mark price
    executable VWAP for a declared quantity

A +20% midpoint excursion means:

> midpoint rose 20%

Nothing more.

It does not imply:

- +20% realizable profit;
- executable entry;
- executable exit;
- available capital;
- successful prediction.

---

## 30. Do Not Call Market Excursion Profit

The Hindsight domain vocabulary SHOULD avoid terminology that implies a trading
counterfactual.

Avoid concepts such as:

    missed profit
    missed trade
    regret
    profit ceiling
    capture percentage
    money left on table
    should-have-entered
    threshold improvement

Prefer:

    ObservedExcursion
    Episode
    ReferencePoint
    SystemSnapshot
    Artifact
    ValidationFinding
    InvariantViolation
    Undefined
    Unavailable
    NotReady

Language is part of the safety boundary of the architecture.

---

## 31. Causal Snapshot

For a ReferencePoint R, Hindsight reconstructs or reads only state causally
available through R.

Future external observations may have been used to select R.

They may not influence `SystemSnapshot(R)`.

Conceptually:

    Episode discovery:
        MAY look after R

    System snapshot:
        MUST NOT look after R

This separation is mandatory.

---

## 32. System Snapshot

A SystemSnapshot is the inspection view assembled for one ReferencePoint.

It should expose, where applicable:

### Capture

    RawFrame
    endpoint
    capture sequence
    receive time
    venue time
    venue sequence
    stream epoch

### Market

    ticker
    trade
    Level3 state
    futures state
    process availability

### Signals / Measurements

    source
    metric
    raw value
    normalized/standardized value
    units
    From
    At
    maturity
    SNR
    readiness
    metadata
    provenance

### Semantic State

    Graph
    Categories
    future Perspectives (when configured)
    Cognition
    Resonance
    Manifold
    Opportunities

### Strategy

    valuation
    alternatives
    planner state
    selected action
    available capital

### Execution / Risk

where applicable:

    requested quantity
    executable quantity
    VWAP
    fees
    spread
    impact
    mark
    stop geometry
    trigger
    realized execution

The purpose is inspection, not judgment by market outcome.

---

## 33. Realism Principle

Hindsight MUST be neither intentionally optimistic nor intentionally pessimistic.

It must use:

> the strongest statement supported by the recorded facts, and no stronger.

When data is sufficient, calculate the quantity.

When data is insufficient, mark it undefined or unavailable.

Never invent conservative or optimistic assumptions merely to force a result.

---

## 34. No Optimistic Execution

Hindsight MUST NOT assume:

- infinite liquidity;
- touch-price fills for arbitrary quantity;
- zero fees;
- zero spread;
- zero impact;
- future depth;
- fills at prices not present in the causal book;
- full execution when recorded depth is insufficient.

---

## 35. No Artificially Pessimistic Execution

Hindsight MUST NOT invent:

- arbitrary slippage;
- arbitrary latency penalties;
- arbitrary liquidity haircuts;
- arbitrary impact multipliers;
- arbitrary failure probability;
- artificial spread widening.

If the historical evidence does not support a penalty, do not fabricate one.

---

## 36. Execution Inspection Is Conditional

A normal market Episode does not require hypothetical execution.

Example:

    observed midpoint excursion = +20%

is already a valid market fact.

Execution inspection becomes appropriate when there is a defensible quantity,
such as:

- an actual order quantity;
- a proposed quantity recorded by strategy;
- a position quantity;
- another explicitly recorded amount.

If no defensible quantity exists:

    hypothetical executable PnL = undefined

Do not invent:

    one coin
    $100
    whole wallet
    10% capital
    maximum available depth

solely to manufacture a counterfactual.

---

## 37. Level3 Execution Validation

When execution mathematics is inspected, historical Level3 state must be
reconstructed causally.

Rules:

1. begin from an observed valid snapshot;
2. apply updates in captured/protocol order;
3. use only state available at or before the inspected boundary;
4. never use future depth;
5. walk the actual recorded side of book;
6. respect requested quantity;
7. distinguish partial from complete execution;
8. apply known fee semantics;
9. report undefined when reconstruction is incomplete.

Prefer the same canonical L3 reduction mathematics used by live/replay rather
than a separate Hindsight-specific BookManager.

---

## 38. Exact Market Move vs Executable Move

Hindsight must distinguish:

    ObservedPriceExcursion

from:

    ExecutableExcursion

The first is market geometry.

The second requires quantity-constrained execution evidence.

They must never share a label implying equivalence.

---

## 39. Inspection Does Not Tune

Hindsight validators must never emit advice such as:

    lower threshold X
    increase weight Y
    loosen gate Z
    increase confidence
    decrease admission requirement
    change Planner admission rule

A finding says what contract failed.

Example:

    liquidity.depth_divergence violated identity with depth_ratio

not:

    loosen liquidity gate to capture this move

---

## 40. Validation

Validation asks whether each quantity and state transition obeyed its own
contract.

Typical questions include:

    Is the value defined?

    Is it finite?

    Is it within its mathematical domain?

    Are the units correct?

    Is the estimator sufficiently supported?

    Does readiness agree with support?

    Is From <= At?

    Did time regress?

    Was future information used?

    Does an algebraic identity hold?

    Does a cross-layer dependency actually exist?

    Was undefined state converted to zero?

    Did the state transition use the correct prior state?

    Does provenance match the exact facts consumed?

---

## 41. Independent Validation

Where practical, validation must be independent of the production implementation
being checked.

Do not validate:

    productionCalculation(x)

by comparing it to:

    productionCalculation(x)

again.

Preferred validators include:

- exact identities;
- separately implemented reference formulas;
- unit analysis;
- domain checks;
- hand-calculable fixtures;
- causal ordering checks;
- independent book walks;
- independent likelihood calculations;
- accounting identities.

---

## 42. Validation Findings

A finding SHOULD have a precise class.

Useful classes include:

    VALID
    UNDEFINED
    NOT_READY
    UNAVAILABLE
    NON_FINITE
    DOMAIN_VIOLATION
    IDENTITY_VIOLATION
    UNIT_VIOLATION
    CAUSALITY_VIOLATION
    TEMPORAL_VIOLATION
    READINESS_VIOLATION
    PROVENANCE_VIOLATION
    STATE_TRANSITION_VIOLATION
    SEMANTIC_VIOLATION
    CROSS_LAYER_VIOLATION
    CAPTURE_INTEGRITY_VIOLATION

`UNDEFINED`, `NOT_READY`, and `UNAVAILABLE` are not automatically defects.

They are defects only when the component contract says the quantity should have
been defined.

---

## 43. Undefined Is Not Zero

Hindsight must preserve:

    zero
    undefined
    unavailable
    immature
    invalid

as different states.

Examples:

    actual zero imbalance
        -> defined zero

    z-score with no estimable dispersion
        -> undefined

    observation not captured
        -> unavailable

    estimator lacking support
        -> not ready

    NaN emitted as a valid metric
        -> invalid

Do not normalize these states into a convenient zero for display.

---

## 44. Downstream Taint

When an upstream fact violates its contract, Hindsight MAY mark downstream
artifacts as:

    depends_on_invalid_state

This means:

> interpretation of downstream state is not trustworthy until the upstream
> violation is understood.

It does NOT mean:

> this defect caused the historical market outcome.

Causal blame for profitability must not be inferred.

---

## 45. Successful and Failed Trades Are Both Inspection Cases

Profit does not prove correctness.

Loss does not prove incorrectness.

A successful trade may reveal:

- invalid accounting;
- future leakage;
- broken maturity;
- incorrect stop geometry.

A losing trade may have perfectly valid:

- signals;
- valuation;
- execution;
- risk mathematics.

Hindsight judges contracts.

Not outcome morality.

---

## 46. Observer Availability

Hindsight must distinguish:

    market existed

from:

    SYMM was observing it

If an Episode begins before process startup, Hindsight must not pretend the
system knew the beginning.

State before observation start is:

    unavailable

unless it was legitimately reconstructed from an explicitly defined replay
initialization process.

---

## 47. Capture Integrity

A Run must expose an explicit capture-integrity state.

At minimum:

    COMPLETE
    GAPPED
    CORRUPT

A gap includes conditions such as:

- missing CaptureSequence;
- raw persistence failure;
- missing payload;
- hash mismatch;
- Envelope referencing absent RawFrame;
- missing Envelope ordinal;
- broken stream epoch;
- witness referencing missing Envelope;
- malformed provenance relationship.

---

## 48. Hindsight Eligibility

Hindsight MUST NOT silently treat incomplete capture as complete evidence.

If an inspected interval crosses a capture gap:

    inspection certainty is broken

and Hindsight must say so.

Example:

    System state unavailable:
    capture gap from sequence 91881 through 91884.

The live trading system may have a separate policy for storage failure.

That is independent.

But Hindsight certainty must fail closed.

---

## 49. Capture Persistence and Live Processing

Capture identity exists before persistence.

Therefore the live system may technically continue processing even if storage
fails.

If it does:

    those Envelopes retain their CaptureRefs

but:

    the capture Run becomes GAPPED

Hindsight may not later claim complete reconstruction across that interval.

This keeps trading-availability policy separate from inspection-integrity policy.

---

## 50. Storage Independence

Hindsight's logical identity model must not depend on SQLite.

SQLite may be the current implementation.

Future storage may be:

    S3-compatible objects
    another database
    append-only files
    another durable engine

The following identities must survive unchanged:

    RunID
    CaptureRef
    EnvelopeRef
    ArtifactID
    StateVersion

Storage row IDs are implementation details.

---

## 51. Logical Storage Model

The persistent inspection model can be understood as four primary record
families:

    Run
    RawFrame
    EnvelopeManifest
    ArtifactWitness

### Run

    RunID
    StartedAt
    CodeCommit
    BuildID
    ConfigDigest
    SchemaVersions
    Integrity

### RawFrame

    CaptureRef
    ReceivedAt
    Endpoint
    Kind
    PayloadHash
    Payload

### EnvelopeManifest

    EnvelopeRef
    Workload
    DomainKind
    Symbol
    VenueAt
    VenueSequence

### ArtifactWitness

    EnvelopeRef
    Boundary
    ArtifactID
    ArtifactKind
    Component
    ComponentStateVersion
    ImmediateParents
    Payload

The implementation may normalize or partition this differently.

The semantics must remain.

---

## 52. Replay Ordering

Replay MUST feed external observations in recorded capture order.

Do not sort the tape by:

    venue timestamp
    ticker timestamp
    trade timestamp
    symbol
    payload kind

before feeding Workspace.

That would reconstruct a market observation order that the original process
never saw.

Protocol-internal ordering inside one raw frame must remain deterministic.

---

## 53. Replay Determinism

Given:

    identical RawFrame tape
    identical initial state
    identical code build
    identical configuration
    identical deterministic seeds where required

semantic replay output should be reproducible.

If parallel scheduling can legitimately affect shared state ordering, the
historical Witness must retain StateVersion evidence of the actual live ordering.

Replay must not silently claim bit-for-bit historical equivalence where the
runtime contract does not guarantee it.

---

## 54. Historical Witness Has Priority

When answering:

> What did SYMM actually believe then?

the historical Witness is authoritative.

Replay is not allowed to overwrite history with what the current implementation
would now calculate.

This distinction is essential when diagnosing bugs.

---

## 55. Episode UI

The Hindsight UI should center the inspected market Episode.

Example:

    BTC/USD

    Observed excursion:
        +20.31%

    Coordinate:
        midpoint

    Span:
        184 observations

    Reference:
        Anchor
        Capture 981882
        Envelope 0
        Venue time ...
        Received time ...

    Capture integrity:
        COMPLETE

    Historical system state:
        ...

    Validation findings:
        ...

The UI should make provenance navigable:

    Artifact
        -> parent artifact
        -> originating Envelope
        -> exact RawFrame

---

## 56. UI Must Not Imply Tuning

Do not display:

    missed profit
    parameter recommendation
    gate to loosen
    threshold to change
    score that should have been higher

Do display:

    observed market outcome
    exact system state
    component readiness
    provenance
    validation findings
    state evolution around reference points

---

## 57. State Evolution

It is useful to inspect:

    before reference
    at reference
    after reference
    peak
    reversal

But each snapshot remains explicitly identified.

Hindsight must not search forward and choose whichever later internal state best
"explains" the eventual market move.

At ReferencePoint R:

    inspect State(R)

A later interesting state is another ReferencePoint.

---

## 58. No Best-Explaining State

Forbidden behavior includes:

    choose highest Opportunity inside the Episode
    choose strongest Perspective
    choose largest score
    choose state nearest eventual direction
    choose decision that makes the narrative easiest

That is hindsight bias inside the Hindsight engine itself.

---

## 59. Reference Episodes Are Not Labels for Learning

Episode labels such as:

    +20% excursion
    reversal
    liquidity shock

must not automatically become production training labels.

Using Hindsight data for an explicit research or learning experiment is a
separate system with a separate specification.

Hindsight itself is inspection.

---

## 60. Performance

The live capture/witness path must respect the Workspace architecture.

Do not restore:

- repeated full Envelope serialization;
- hidden queues;
- unbounded recorder buffers;
- Envelope retention;
- cloned world snapshots.

Observers should encode the artifact they witness and return.

They must not retain the ring-owned Envelope.

---

## 61. Backpressure

If durable capture or witnessing is synchronous, its backpressure is real and
must be visible.

If asynchronous persistence is introduced, it must use:

    bounded capacity
    explicit overflow semantics
    explicit integrity failure

Never silently drop Hindsight evidence.

A dropped observation marks the Run GAPPED.

---

## 62. No Hindsight Book Manager

Hindsight must not recreate a giant retained market-event database in memory
simply to ask historical state questions.

Raw history lives in durable capture.

Replay streams it.

Resident reducers retain only the current bounded state required by their
mathematics.

Events move.

State stays.

---

## 63. Validation Workflow

A normal inspection workflow is:

    1. choose Capture Run

    2. verify capture integrity

    3. discover market Episode

    4. identify ReferencePoint

    5. load Historical Witness around ReferencePoint

    6. build SystemSnapshot

    7. run independent validators

    8. inspect findings and provenance

Optionally:

    9. replay the same raw tape through a specified corrected build

    10. validate that build separately

At no point does the workflow generate strategy-tuning recommendations.

---

## 64. Example

Suppose Hindsight discovers:

    BTC/USD
    midpoint rose 20% over 187 ticker observations

and identifies Capture 120000 as the retrospective anchor.

Hindsight asks:

    What raw input was Capture 120000?

    Which Envelope(s) came from it?

    What signal outputs existed at that exact boundary?

    What resident historical state participated?

    Which Category state existed?

    Which Perspectives existed?

    Was an Opportunity present?

    What did valuation know?

    What did the planner do?

    Were all of those calculations valid?

Suppose it finds:

    liquidity.depth_ratio = 1.4142
    liquidity.depth_divergence = 0.6931

Those are consistent:

    log(1.4142) ~= 0.3466

Therefore if the specification says:

    depth_divergence = log(depth_ratio)

this is an identity violation.

Hindsight reports:

    IDENTITY_VIOLATION
    liquidity.depth_divergence

It does NOT report:

    this bug cost us 20%

and it does NOT recommend:

    change liquidity threshold to X.

---

## 65. Required Adversarial Tests

### 65.1 Raw-to-Envelope Identity

Capture one raw frame producing multiple semantic records.

Assert:

    all Envelopes carry the same CaptureRef
    each has a unique deterministic ordinal

Mutation removing the origin propagation must fail.

---

### 65.2 Zero-Envelope Raw Frame

Capture heartbeat/status input.

Assert the RawFrame exists even when no semantic Envelope is produced.

---

### 65.3 Timestamp Collision

Create two different captured inputs with identical venue timestamps.

Hindsight must distinguish them by CaptureRef.

---

### 65.4 Clock Skew

Create:

    ReceivedAt < VenueAt

and another:

    ReceivedAt > VenueAt

Neither may break identity or reorder replay.

---

### 65.5 Replay Ordering

Give venue timestamps deliberately out of local receive order.

Replay must follow CaptureSequence.

Sorting by venue time must fail the test.

---

### 65.6 Exact Artifact Provenance

Produce a Measurement from one Envelope.

Assert:

    Measurement witness
        -> exact EnvelopeRef
        -> exact RawFrame

No timestamp search is allowed.

---

### 65.7 Stateful Parent Provenance

Have a future named-question Advisor combine current ticker state with retained
earlier CVD and DepthFlow.

Assert all three exact origins remain visible.

---

### 65.8 Cross-Workload State Version

Advance the same shared semantic state from two Workloads.

Assert monotonically ordered StateVersions record actual transition order.

---

### 65.9 Future Leakage

Choose an Episode anchor using future market data.

Change all post-anchor inputs.

The Historical SystemSnapshot at the anchor must remain unchanged.

---

### 65.10 Capture Gap

Remove one raw CaptureSequence from an otherwise valid run.

Run integrity becomes GAPPED.

Hindsight must refuse to claim complete state across that interval.

---

### 65.11 Missing Witness

Remove an ArtifactWitness referenced by a downstream artifact.

Validation must surface provenance/integrity failure.

It must not silently reconstruct a convenient substitute.

---

### 65.12 Profitable but Invalid

Construct a profitable historical execution containing an invalid accounting
identity.

Hindsight reports the violation.

Profit does not suppress it.

---

### 65.13 Losing but Valid

Construct a losing historical execution whose mathematics satisfies every
declared contract.

Hindsight must not invent a validation failure because PnL was negative.

---

### 65.14 No Counterfactual Quantity

Discover a large market excursion with no actual/proposed quantity.

Executable hypothetical return must remain undefined.

---

### 65.15 L3 Future Leak

When validating execution at capture N, provide much better depth at capture
N+1.

Execution at N must not see it.

---

### 65.16 Insufficient L3 Depth

Request quantity greater than causally available depth.

Report incomplete/undefined full execution.

Do not fill the remainder at touch.

---

### 65.17 Witness vs Replay

Store an intentionally incorrect historical Measurement.

Replay with corrected code produces a different value.

Historical Witness must continue showing the original incorrect value.

Replay must show the corrected value under a separate Replay Run.

---

### 65.18 Mutation Kills

Every important validator must demonstrate a defect that makes its test red.

A green validator that has never demonstrated detection of its target defect is
not trusted merely because its tests pass.

---

## 66. Architectural Anti-Patterns

The following violate the Hindsight design:

    correlate artifacts by nearest timestamp

    sort raw capture by venue time before replay

    serialize every full Envelope at every boundary

    retain every Level3 event in memory for later reconstruction

    invent hypothetical trade quantity

    use future book state

    convert undefined to zero

    call observed market excursion profit

    call CASH before a rally a mistake

    recommend threshold changes from historical outcome

    let replay overwrite historical witness

    use SQLite row ID as permanent domain identity

    silently ignore capture gaps

    select the strongest later state to explain an earlier Episode

    use Hindsight output as a production control input

---

## 67. Success Criteria

The replacement Hindsight system is correctly founded when:

1. every raw external input has a stable CaptureRef assigned before parsing;

2. every semantic Envelope is deterministically linked to its exact raw input;

3. one raw frame may map cleanly to zero, one, or many Envelopes;

4. every important derived artifact is traceable through immediate provenance;

5. stateful cross-Workload consumers expose the actual state version involved;

6. historical Witness and Replay are distinct data domains;

7. market Episode discovery is independent of trading decisions;

8. future data may choose a ReferencePoint but cannot contaminate its snapshot;

9. execution claims are quantity- and L3-grounded where execution is inspected;

10. missing execution evidence remains undefined;

11. capture gaps make inspection uncertainty explicit;

12. no component emits tuning recommendations;

13. profitable outcomes do not hide mathematical defects;

14. losing outcomes do not manufacture defects;

15. storage backend identity is irrelevant to causal identity;

16. replay consumes the exact captured external sequence;

17. Hindsight can navigate:

        validation finding
            -> artifact
            -> parent artifact(s)
            -> Envelope
            -> exact RawFrame

without using fuzzy timestamp correlation.

---

## 68. Final Definition

Hindsight is a microscope over a captured running system.

The market tells us which slides are interesting.

Capture tells us what reality reached SYMM.

Witnesses tell us what SYMM actually produced.

Provenance tells us exactly why each fact existed.

Validators tell us whether the mathematics satisfied its own contracts.

Replay lets us inspect corrected machinery against the same historical reality.

Nothing in Hindsight says:

> tune this and get fat stacks.

The strongest statement it is allowed to make is:

> At this exact historical boundary, under these exact causally available
> observations, this component was valid, invalid, undefined, not ready, or
> unavailable — and here is the evidence.

# Workspace Runtime Specification

## 1. Purpose

Workspace is SYMM's real-time streaming execution fabric.

It is not a pub/sub bus.

It is not a topic router.

It is not a recursive event dispatcher.

It does not publish outputs from one node back into itself.

Instead, Workspace owns a small set of statically configured Workloads.

Each Workload represents one complete execution graph for one class of ingress
event.

An ingress value enters a Workload exactly once.

That value occupies one ring slot and advances through an ordered sequence of
Disruptor HandlerGroups.

Within a HandlerGroup, independent Nodes execute concurrently.

Between HandlerGroups, the Disruptor provides a barrier: every Node in the
previous group must complete before any Node in the next group may observe the
slot.

The central runtime model is:

    one ingress event
        -> one ring reservation
        -> one Envelope
        -> one trip through one Workload
        -> parallel computation where independent
        -> barriers where dependent
        -> observation at explicit boundaries
        -> slot released for reuse

No republishing is required.

The HandlerGroups ARE the fanout.

The barriers ARE the join.

---

## 2. Core Invariant

The Workspace execution model is summarized by:

> One event. One slot. One trip through the graph.

An event MUST NOT be copied into intermediate queues merely to move between
semantic stages.

A Node MUST NOT publish its result back into Workspace.

A downstream Node observes upstream results by reading the same Envelope after
the Disruptor barrier has established completion.

---

## 3. Terminology

### 3.1 Workspace

A Workspace owns the configured Workloads and their lifetime.

Conceptually:

    Workspace
        -> Ticker Workload
        -> Trade Workload
        -> Level3 Workload
        -> Execution Workload
        -> other explicitly configured ingress Workloads

Workspace is responsible for routing a newly arriving Envelope to the correct
Workload.

It does not route intermediate computation results.

---

### 3.2 Workload

A Workload is one Disruptor instance plus its preallocated ring storage and
ordered HandlerGroups.

A Workload represents a complete execution graph for one ingress class.

Example:

    Ticker Workload

        Group 1:
            Correlation
            LeadLag
            Liquidity

        Group 2:
            ObserveAfterSignals

        Group 3:
            Category

        Group 4:
            ObserveAfterCategory

The Workload's ring is the transport for the entire computation.

---

### 3.3 Envelope

An Envelope is the mutable domain object travelling through a Workload.

It contains:

1. the ingress event;
2. semantic outputs produced by Nodes;
3. downstream artifacts derived from those outputs.

Example:

    Envelope {
        TypeID

        TickerData
        TradeData
        Level3Data

        Correlation
        LeadLag
        Liquidity
        CVD
        DepthFlow
        Morphology
        Hawkes
        ...

        Categories
        Opportunities
        GraphUpdate
        Resonance
        Manifold
        Cognition
        CausalOutput
        ...
    }

The Envelope is not itself a message bus.

It is the state of one computation as it advances through the graph.

---

### 3.4 Node

A Node is one semantic processor inside a Workload.

A Node consumes the current Envelope state and may write only the output state
it owns.

A Node does not publish.

A Node does not route.

A Node does not decide which Node executes next.

The Workload graph determines execution order.

---

### 3.5 HandlerGroup

A HandlerGroup is a set of Nodes that may execute concurrently.

For:

    {
        Correlation,
        LeadLag,
        Liquidity,
    }

the runtime means:

                     Envelope
                         |
             +-----------+-----------+
             |           |           |
        Correlation   LeadLag    Liquidity
             |           |           |
             +-----------+-----------+
                         |
                      barrier

The group itself is the fanout.

There is no separate fanout abstraction.

---

### 3.6 Barrier

The boundary between HandlerGroups is a synchronization barrier supplied by the
Disruptor.

If Group N precedes Group N+1:

    Group N
        -> barrier
        -> Group N+1

then every write performed by Group N is complete and visible before Group N+1
begins processing that slot.

This is the dependency model of Workspace.

---

## 4. Static Execution Graph

The execution graph MUST be declared when the Workload is constructed.

Example:

    NewWorkload(
        ctx,
        [][]Node[*types.Envelope]{
            {
                correlation.NewSignal(ctx),
                leadlag.NewSignal(ctx),
                liquidity.NewSignal(ctx),
            },
            {
                category.NewSolver(ctx),
            },
        },
    )

means:

    Ticker
      |
      +--> Correlation --+
      +--> LeadLag ------+--> barrier --> Category
      +--> Liquidity ----+

Workspace MUST NOT dynamically discover downstream consumers from runtime types.

Workspace MUST NOT recursively inspect returned values and dispatch them again.

The graph is configuration, not runtime inference.

---

## 5. Ingress Routing

Only values entering from outside an existing Workload require routing.

Typical ingress sources include:

- websocket market events;
- exchange execution events;
- system-control events;
- replayed captured events.

An ingress event is wrapped in an Envelope and routed exactly once to the
appropriate Workload.

Example:

    TickerData
        -> EnvelopeTicker
        -> tickerWorkload.Push(envelope)

    TradeData
        -> EnvelopeTrade
        -> tradeWorkload.Push(envelope)

    Level3Data
        -> EnvelopeLevel3
        -> level3Workload.Push(envelope)

Once inside the Workload, Workspace performs no further semantic routing.

---

## 6. No Publish Model

There is no general-purpose:

    Publish(value)

operation for intermediate results.

This pattern is forbidden:

    Signal.Step(Ticker)
        -> Measurement
        -> Workspace.Publish(Measurement)
        -> Category.Step(Measurement)
        -> Category
        -> Workspace.Publish(Category)

The correct model is:

    Envelope enters ring

    Signal:
        envelope.Liquidity = measurement

    barrier

    Category:
        reads envelope.Liquidity

The return value of a Node is therefore not a new Workspace event.

The Envelope is the continuation.

---

## 7. Fanout and Join

Workspace MUST rely on Disruptor HandlerGroups for fanout and dependency joins.

Parallel independent work belongs in one group.

Dependent work belongs in a later group.

Example:

    Group 1:
        Correlation
        LeadLag
        Liquidity

    Group 2:
        Category

Group 1 may execute concurrently.

Group 2 may begin only after all Group 1 handlers have completed the slot.

No manual WaitGroup, semaphore, completion counter, or fanout channel should be
added to reproduce behavior already supplied by the Disruptor graph.

---

## 8. Envelope Ownership

Concurrency safety is achieved through explicit field ownership.

Nodes executing in the same HandlerGroup MUST NOT concurrently mutate the same
semantic state.

For example:

    Correlation owns:
        Envelope.Correlation

    LeadLag owns:
        Envelope.LeadLag

    Liquidity owns:
        Envelope.Liquidity

Those three fields may therefore be written concurrently.

The following is forbidden:

    Group:
        Node A appends to Envelope.Measurements
        Node B appends to Envelope.Measurements
        Node C appends to Envelope.Measurements

because the slice header and backing storage become shared mutable state.

Prefer explicit ownership:

    Envelope.Correlation
    Envelope.LeadLag
    Envelope.Liquidity

over generic concurrent result containers.

---

## 9. Read and Write Rules

For a Node in Group N:

It MAY:

    read immutable ingress facts;
    read outputs committed by groups < N;
    write fields exclusively owned by itself in Group N.

It MUST NOT:

    mutate another Node's owned output;
    mutate output belonging to a later group;
    retain a pointer to the Envelope after Handle returns;
    asynchronously mutate the Envelope;
    hand the Envelope to another goroutine;
    place the Envelope into another queue.

Within one HandlerGroup, concurrent Nodes must have disjoint write ownership.

After the group barrier, later Nodes may read all completed outputs.

---

## 10. Slot Lifetime

The ring owns the lifetime of the Envelope reference while that sequence is in
flight.

Conceptually:

    Reserve
       |
       v
    slot = Envelope
       |
       v
    Group 1
       |
    barrier
       |
    Group 2
       |
    barrier
       |
      ...
       |
    final handler completes
       |
       v
    slot eventually reusable

No Node may assume the Envelope remains valid after processing of its ring
sequence has completed.

A Node MUST NOT retain:

    *Envelope

or any mutable object owned exclusively by that Envelope unless ownership is
explicitly transferred out of the ring.

---

## 11. Compute Groups

A Compute Group changes semantic state.

Examples:

    signals
    category
    relationship reasoning
    opportunity
    valuation
    cognition
    planner
    execution calculations
    position-risk calculations

Compute Nodes may mutate their owned Envelope outputs.

A Compute Group should contain the maximal set of Nodes that:

1. depend on the same previous barrier;
2. are mutually independent;
3. write disjoint state.

This maximizes concurrency without inventing synchronization.

---

## 12. Observation Groups

Workspace MAY place an Observation Group between semantic Compute Groups.

Example:

    Compute: Signals
        |
    barrier
        |
    Observe: Signal boundary
        |
    barrier
        |
    Compute: Category
        |
    barrier
        |
    Observe: Category boundary

Observation Nodes perform external observation of the completed semantic state.

Typical purposes include:

    UI publication
    telemetry
    diagnostics
    recording
    tracing
    Hindsight capture
    validation capture

Observation is downstream of computation.

Observation MUST NOT influence computation.

---

## 13. Observation Invariant

The core Observation invariant is:

> Observation may report semantic state.
> Observation may never create semantic state required by later computation.

An Observation Node MUST NOT:

- alter strategy state;
- change a measurement;
- modify readiness;
- modify Opportunity state;
- change Valuation;
- influence Planner/MCTS;
- write market-derived semantic facts;
- provide an input that a later Compute Group requires.

The next Compute Group must behave identically whether UI, telemetry, or
recording is enabled or disabled.

---

## 14. Barrier Snapshots

An Observation Group runs after a complete semantic barrier.

Therefore it observes an exact causal boundary.

For:

    Signals
        -> ObserveSignals
        -> Category
        -> ObserveCategory
        -> Opportunity
        -> ObserveOpportunity

the observation points mean precisely:

    after every Signal in this stage completed

    after Category completed

    after Opportunity completed

There is no ambiguity about partially written state.

This is the preferred basis for:

- runtime diagnostics;
- recording;
- Hindsight state inspection;
- performance attribution;
- UI state publication.

---

## 15. Hindsight Compatibility

The Workspace architecture should make Hindsight observation natural.

Hindsight does not need to infer approximately what state existed between
components.

Explicit barriers already define those moments.

A capture may record selected facts at boundaries such as:

    ingress
    after-signals
    after-category
    after-relationships
    after-opportunity
    after-valuation
    after-planner
    after-execution
    after-position-risk

Recording a boundary does not require cloning the entire Envelope.

The observer should encode only the state required by the capture contract while
the Envelope is valid.

The observer MUST NOT retain the Envelope itself.

---

## 16. Observation Cost and Backpressure

An Observation Group is a real HandlerGroup.

It participates in the Workload dependency graph.

Therefore:

> A slow observer delays downstream computation.

This is intentional and MUST NOT be disguised.

Workspace MUST NOT solve slow telemetry by silently adding:

    buffered channels
    hidden queues
    mailboxes
    unbounded slices
    background backlogs

If an external system requires asynchronous delivery, that delivery mechanism
must have an explicit bounded capacity and explicit overload semantics outside
the semantic Workspace graph.

Backpressure must remain visible.

---

## 17. Recording Semantics

Recording policy must be explicit.

If recording is required for correctness or reproducibility, the recorder may
remain synchronous and therefore exert backpressure.

If recording is allowed to lose observations under overload, that must be an
explicit recorder policy.

It MUST NOT silently drop because an undocumented internal queue filled.

A recorded boundary represents state that genuinely existed after its preceding
Compute Group completed.

---

## 18. UI Semantics

UI publication belongs in Observation Groups.

UI is a view of completed semantic state.

UI code MUST NOT:

    own semantic state;
    become a dependency for a later Compute Group;
    mutate the Envelope;
    determine runtime correctness.

A disconnected or slow UI must not change what the trading system believes.

Any buffering required for remote UI transport belongs after the Workspace
observation boundary with explicit overload behavior.

---

## 19. Telemetry Semantics

Telemetry reports execution.

It does not participate in execution.

Useful telemetry at each boundary may include:

    workload
    group
    sequence
    event type
    event key/symbol
    processing duration
    observer duration
    saturation/backpressure
    node failures

Instrumentation MUST NOT create hidden scheduling behavior inside semantic
Nodes.

---

## 20. Level 3 Market Data

Level3 is a first-class Workspace ingress event.

The Workspace architecture MUST NOT require a parallel global order-book manager
merely to notify consumers that the book changed.

The model is:

    Kraken Level3
        -> decode
        -> EnvelopeLevel3
        -> Level3 Workload

Independent Level3 consumers belong in the same HandlerGroup where possible.

Example:

    Group 1:
        DepthFlow
        Morphology
        ExecutableDepth
        other independent L3 measurements

Each consumer sees the same Level3 event exactly once.

Each maintains only the resident state required by its own mathematics.

There is no separate semaphore saying:

    "the book changed"

The event itself is the change.

---

## 21. Resident State

Events move.

State stays.

A Node may retain bounded resident state required to interpret future events.

Examples:

    Hawkes retained arrival path
    previous Morphology state
    current executable depth
    historical estimator state
    current Opportunity state

A Node MUST NOT retain an unbounded history merely because events are available.

The Workspace does not provide a global history service.

Historical retention belongs to the mathematical component that requires it and
must obey that component's specification.

---

## 22. Reusable Stateful Reducers

When multiple consumers genuinely require identical state-transition semantics,
the implementation SHOULD extract reusable pure/stateful reduction mathematics
rather than create a shared global manager.

For example:

    current L3 state + Level3Data
        -> next L3 state

may be reusable between:

    live execution
    PositionRisk
    replay
    Hindsight validation

The shared concept is the transition semantics.

The shared concept is NOT necessarily a globally mutable singleton.

---

## 23. Ordering

Within one Workload, committed sequences are processed in ring order.

For one ingress stream:

    sequence N
    sequence N+1
    sequence N+2

must preserve the Workload's configured ordering semantics.

Nodes MUST NOT reorder events by spawning asynchronous semantic work.

Same-event parallelism is provided by HandlerGroups.

Event ordering remains owned by the Workload.

---

## 24. Writers

The Disruptor WriterCount MUST match the actual ingress topology.

If exactly one goroutine writes a Workload:

    WriterCount(1)

is correct.

If multiple goroutines concurrently call Push on the same Workload, the
Disruptor MUST be configured accordingly.

The configured writer count is a concurrency fact, not a performance tuning
guess.

---

## 25. Ring Capacity

Ring capacity must satisfy the Disruptor's power-of-two requirements.

The mask is:

    mask = capacity - 1

and slot access is:

    buffer[sequence & mask]

The ring is bounded.

When downstream processing cannot keep up, producers experience backpressure.

Workspace MUST NOT grow another unbounded structure to avoid this condition.

---

## 26. No Hidden Backlog

There must be no hidden backlog after the Workload ring.

Forbidden examples include:

    []Work pending queues
    buffered semantic channels
    per-symbol mailboxes
    arbitrary retry queues
    asynchronous Envelope queues
    observer-owned Envelope queues

The Disruptor is the bounded queue.

Do not build another one behind it.

---

## 27. Error Semantics

Errors must be explicit.

A Workload or Node must not silently corrupt the Envelope and allow later stages
to treat partial state as valid.

If a Node fails to produce an output:

    the output remains absent

unless its domain explicitly defines another state.

Undefined is not zero.

An infrastructure failure that makes continued Workload processing unsafe must
surface through the Workload/Workspace error path.

Workspace must not fabricate downstream results to preserve apparent liveness.

---

## 28. Cancellation and Shutdown

Workspace owns the lifetime of its Workloads.

Closing Workspace:

    cancels Workspace context
    closes Workloads
    stops Disruptors
    waits for accepted ring work according to Disruptor close semantics
    releases runtime resources

Shutdown must not create a second drain queue.

Nodes should honor context cancellation where relevant.

---

## 29. Configuration Is the Graph

The Workload declaration is executable architecture documentation.

For example:

    NewWorkload(
        ctx,

        Stage(
            correlation,
            leadlag,
            liquidity,
        ),

        Observe(
            telemetry,
            recorder,
            ui,
        ),

        Stage(
            category,
        ),

        Observe(
            telemetry,
            recorder,
            ui,
        ),
    )

expresses:

    Correlation ─┐
    LeadLag ─────┼─> barrier
    Liquidity ───┘

                     ↓

                Observation

                     ↓

                  Category

                     ↓

                Observation

There should be no separate hidden dependency graph that disagrees with this
configuration.

---

## 30. Stage and Observe Constructors

The runtime MAY provide configuration helpers such as:

    Stage(nodes...)
    Observe(nodes...)

for readability.

These do not introduce new scheduling machinery.

Both ultimately become Disruptor HandlerGroups.

Their distinction exists to express semantic intent and enable validation of the
different mutation rules.

For example:

    Stage(...)
        semantic mutation allowed under field ownership rules

    Observe(...)
        semantic mutation forbidden

---

## 31. Workload Examples

### 31.1 Ticker

    Ingress: TickerData

    Stage:
        Correlation
        LeadLag
        Liquidity

    Observe

    Stage:
        Category

    Observe

    Stage:
        downstream reasoning as applicable

    Observe

---

### 31.2 Trade

    Ingress: TradeData

    Stage:
        CVD
        Hawkes
        other independent trade measurements

    Observe

    Stage:
        downstream consumers

    Observe

---

### 31.3 Level3

    Ingress: Level3Data

    Stage:
        DepthFlow
        Morphology
        executable-depth state
        other independent book measurements

    Observe

    Stage:
        downstream consumers requiring completed L3 interpretations

    Observe

No external "book changed" semaphore is required.

---

## 32. Determinism

Given:

    identical ingress event sequence
    identical initial resident state
    identical Workload graph
    identical configuration

semantic outputs must be reproducible.

Parallel Nodes within one HandlerGroup may execute in any physical order, but
their outputs must be independent such that scheduling order cannot change the
result.

If execution ordering between two Nodes changes semantics, they do not belong in
the same HandlerGroup.

Place them in separate groups.

---

## 33. Testing the Graph

Runtime tests must verify the actual scheduling contract.

At minimum:

### Parallel fanout

Two Nodes in one group receive the same sequence and may execute concurrently.

### Barrier ordering

A Node in Group 2 never sees the slot until every Node in Group 1 has completed.

### Disjoint writes

Concurrent Nodes populate independent Envelope fields without races.

### Same slot

Downstream Nodes observe the exact same Envelope instance/state advanced by
upstream Nodes rather than a clone or republished copy.

### Ordering

Ingress sequences preserve Workload order.

### Backpressure

A stalled downstream HandlerGroup eventually prevents unlimited producer
progress; no hidden queue absorbs the work.

### Observation read-only

An observation handler attempting to alter semantic state must be considered an
architectural violation.

### No pointer escape

Tests/review should detect observers or Nodes retaining ring-owned Envelope
references after Handle returns.

### Determinism

Different scheduling orders inside a parallel group produce identical semantic
results.

---

## 34. Architectural Anti-Patterns

The following patterns violate the Workspace design:

    Node -> Publish -> Workspace -> Node

    Signal -> buffered channel -> Category

    L3 -> global BookManager -> semaphore -> signal

    observer -> semantic state mutation

    concurrent Nodes appending to one shared results slice

    Node retains Envelope pointer after Handle

    Node launches goroutine that later mutates Envelope

    downstream work hidden behind an async queue

    runtime type discovery used to recursively route intermediate results

    duplicate per-stage copies of the same event

    global shared mutable state used only to avoid expressing a dependency
    through HandlerGroups

---

## 35. Performance Model

The design is intended to minimize:

    allocations
    copies
    scheduler involvement
    synchronization
    cache movement
    hidden queues

One ingress event should normally require:

    one Envelope
    one ring reservation
    one sequence traversal

Parallel semantic work operates over that same slot.

Later stages consume already-produced fields directly.

Workspace should not allocate intermediate result envelopes merely to cross a
stage boundary.

---

## 36. Architectural Consequence

Workspace is not a message broker.

It is a statically composed parallel execution pipeline.

The Envelope is not a collection of messages.

It is one travelling computation.

HandlerGroups are not arbitrary batches of subscribers.

They are the parallel layers of the computation graph.

Barriers are not implementation details.

They are the causal boundaries of the system.

Observation layers are not another semantic pipeline.

They are witnesses placed at those causal boundaries.

---

## 37. Core Laws

The Workspace architecture is governed by these laws:

### Law 1 — One Event, One Trip

An ingress event enters one Workload once.

### Law 2 — The Ring Is the Queue

No hidden semantic backlog exists behind it.

### Law 3 — Groups Are Fanout

Independent Nodes execute concurrently in one HandlerGroup.

### Law 4 — Barriers Are Join

Dependent computation starts only after every required upstream Node completes.

### Law 5 — Disjoint Concurrent Writes

Nodes in the same HandlerGroup own separate semantic outputs.

### Law 6 — The Envelope Is the Continuation

Results move downstream by remaining on the same Envelope, not by republishing.

### Law 7 — Observation Is Read-Only

UI, telemetry, recording, tracing, and diagnostics may witness semantic state
but may not influence it.

### Law 8 — Events Move, State Stays

Components retain only the bounded resident state required by their mathematics.

### Law 9 — Backpressure Is Real

Slow downstream work must not be concealed behind an unbounded queue.

### Law 10 — Configuration Is Truth

The declared Workload HandlerGroups are the execution graph.

---

## 38. Summary

The complete model is:

    ingress
       |
       v
    reserve ring slot
       |
       v
    Envelope
       |
       v
+--------------------------+
| Compute Group N          |
| A | B | C                |
+--------------------------+
         |
         barrier
         |
+--------------------------+
| Observe Group N          |
| UI | telemetry | record  |
+--------------------------+
         |
         barrier
         |
+--------------------------+
| Compute Group N+1        |
+--------------------------+
         |
         ...
         |
   sequence complete
         |
   slot reusable

The central theorem is:

> One event enters one bounded ring and becomes one travelling computation.
> HandlerGroups provide fanout, barriers provide dependency ordering, and
> observers witness completed state between semantic stages.

There is no publish loop.

There is no hidden graph.

There is no second queue.

There is no side-channel required merely to communicate that state changed.

The event is the change.
# Runtime Workspace Specification

## Status

Normative specification for `nomagique/runtime`.

Where the implementation and this specification disagree, this specification defines the intended architecture.

---

## 1. Purpose

The runtime Workspace is a **data-management substrate**.

Its primary responsibility is to provide efficient data routing between independently implemented components while abstracting concurrency, scheduling, buffering, and execution ownership away from those components.

A component connected through the Workspace SHOULD reason in terms of:

```text
receive value
process value
optionally publish value
```

It SHOULD NOT need to reason about:

```text
goroutine ownership
worker lifetime
queue ownership
per-symbol locks
fan-out synchronization
scheduler coordination
```

The Workspace additionally provides:

1. a shared-object pool for runtime state that lives outside the components consuming it;
2. a signaling layer for reactive event notification;
3. an observer layer for side effects that should not participate in the primary data path;
4. failure propagation and runtime diagnostics;
5. a quiescence boundary for deterministic settlement in tests and replay.

The Workspace does not interpret the data it transports.

---

## 2. Core Principle

The governing rule is:

> Components own computation. The Workspace owns execution.

A producer determines **what value exists**.

A consumer determines **what to do with that value**.

The Workspace determines:

- how the value reaches consumers;
- how fan-out occurs;
- where temporary buffering lives;
- when work becomes runnable;
- which worker executes it;
- how ordering is preserved;
- when execution ownership is released.

Concurrency is therefore a property of the data-routing substrate rather than of each connected component.

---

## 3. Workspace Topology

One Workspace represents one connected runtime data plane.

```text
                     Workspace
                         │
        ┌────────────────┼────────────────┐
        │                │                │
    Channels         Shared State      Signals
        │                │                │
        │           Share / Shared     On / Notify
        │
        ├── subscriptions
        │
        ├── keyed lanes
        │
        └── bounded rings
                │
                ▼
         shared worker pool

                         │
                         ▼
                     Observers
                         │
            UI / telemetry / audit /
             capture / other effects
```

Components SHOULD communicate through named channels rather than directly coordinating execution with one another.

---

## 4. Channels

A channel is a named, typed stream of values.

A channel definition consists logically of:

```text
Name
Value type
Key function
```

The key function assigns each published value to an execution lane.

Examples:

```text
ticker      → ticker.Symbol
trade       → trade.Symbol
measurement → measurement.Symbol
global UI   → one global key
```

All users of one named channel MUST agree on its value type and key semantics.

A channel name MUST NOT acquire different scheduling semantics depending on which component happens to request it first.

The channel is transport infrastructure. It MUST NOT attach domain interpretation to the values it carries.

---

## 5. Fan-Out

Publishing one value to a channel delivers that value independently to every subscription on that channel.

Given:

```text
              ticker BTC/USD
                    │
             ChannelTickers
          ┌─────────┼─────────┐
          ▼         ▼         ▼
      liquidity  resonance   desk
```

each subscription receives its own delivery.

One slow consumer MUST NOT require another consumer to share its queue or processing position.

For this reason buffering belongs to the subscription lane, not globally to the channel.

---

## 6. Subscription Lanes

For each:

```text
(subscription, key)
```

the Workspace maintains an independent bounded lane.

Conceptually:

```text
Channel
  │
  ├── Subscription A
  │      ├── BTC/USD → Ring
  │      ├── ETH/USD → Ring
  │      └── SOL/USD → Ring
  │
  └── Subscription B
         ├── BTC/USD → Ring
         ├── ETH/USD → Ring
         └── SOL/USD → Ring
```

A lane is the unit of scheduling and sequential execution.

Values belonging to one lane MUST be processed in FIFO order.

At most one drain for one lane may execute at a time.

Independent lanes MAY execute concurrently.

---

## 7. Concurrency Contract

The Workspace MUST preserve sequential execution within one key while allowing independent keys to progress concurrently.

For a symbol-keyed stream:

```text
BTC 1
BTC 2
BTC 3
```

the consumer MUST observe:

```text
BTC 1 → BTC 2 → BTC 3
```

for retained values.

Meanwhile:

```text
BTC
ETH
SOL
```

MAY execute concurrently.

This guarantee allows a keyed consumer to maintain per-key mutable estimator state without introducing synchronization solely to protect concurrent calls for the same key.

The Workspace MUST NOT require every component to create its own goroutine or mutex topology.

---

## 8. Runnable Work

A lane does not permanently own a worker.

A lane becomes runnable when data is present and no drain already owns responsibility for that lane.

Conceptually:

```text
value published
      │
      ▼
 value enters ring
      │
      ▼
is a drain already responsible?
      │
   ┌──┴──┐
  yes    no
   │      │
   │      ▼
   │   schedule drain
   │
   └──────────────► existing drain consumes it
```

Repeated writes to an already-active lane MUST NOT create one worker task per value.

One scheduled drain owns responsibility for consuming that lane until it reaches an idle boundary.

---

## 9. Worker Pool

All runnable lane drains execute on a shared worker pool.

Workers are execution capacity. They do not belong to channels, subscriptions, symbols, or components.

```text
active lane ─┐
active lane ─┼──► shared worker pool
active lane ─┘

idle lane ──────► no worker ownership
idle lane ──────► no worker ownership
```

When a drain completes, its worker is free to execute other runnable work.

The pool MAY retain idle workers according to its own elasticity policy, but an idle lane MUST NOT retain a dedicated goroutine.

This distinction is fundamental:

> Worker lifetime is a pool concern. Lane lifetime is a data concern.

The number of possible lanes may therefore be much greater than the number of active worker goroutines.

---

## 10. Drain Lifecycle

A drain consumes retained values from one lane sequentially.

Its lifecycle is:

```text
scheduled
   │
   ▼
drain next value
   │
   ▼
execute subscription step
   │
   ├── more values ──► continue
   │
   └── empty
          │
          ▼
    release ownership
          │
          ▼
    verify still empty
       ┌──┴──┐
      yes    no
       │      │
       ▼      ▼
    return   reacquire
```

The empty/recheck transition MUST prevent a value arriving during ownership release from becoming stranded without a future drain.

Once the lane is genuinely idle, the drain returns and releases its worker.

---

## 11. Bounded Retention

Streaming lanes are bounded.

A consumer that cannot keep pace MUST NOT create an indefinitely growing historical backlog.

When a lane exceeds its retention capacity, the oldest retained values MAY be overwritten so the lane continues representing the freshest available stream.

The runtime MUST make such loss observable.

At minimum diagnostics SHOULD preserve:

- submitted values;
- completed values;
- retained pending values;
- lane capacity;
- high-water mark;
- dropped values;
- active drains.

The overload policy is therefore intentionally biased toward:

```text
current state
```

rather than:

```text
unbounded processing of stale state
```

A bounded streaming lane is not a durable event log.

Components requiring lossless historical retention MUST use an appropriate persistence mechanism.

---

## 12. Shared Object Pool

The Workspace provides a shared-object pool for state that is authoritative at runtime but does not belong to the consuming components.

```text
producer
   │
 Share("book", object, symbol)
   │
   ▼
Workspace
   │
 Shared("book", symbol)
   │
   ├──► signal A
   ├──► signal B
   └──► signal C
```

A shared object allows consumers to access common state without importing or coordinating directly with the component that produced it.

Examples include:

- authoritative order books;
- cross-sectional state;
- other runtime resources whose identity is shared across several components.

`Share` / `Shared` is a state facility, not a stream.

Publishing a new channel value and mutating a shared object are distinct operations.

Consumers MUST NOT infer that a shared object changed merely because they hold a reference to it.

---

## 13. Signaling

The Workspace provides lightweight reactive signaling through named trigger topics.

```text
producer
   │
 Notify("disconnect")
   │
   ▼
Workspace
   │
   ├──► listener A
   └──► listener B
```

Signals communicate:

> Something happened; react.

They do not carry the primary analytical data stream.

Typical uses include lifecycle or invalidation events such as:

- connection loss;
- resource invalidation;
- refresh requests;
- shared-state change notification.

A signal listener SHOULD NOT need to know which concrete component emitted the event.

The signaling layer MUST NOT be used as a substitute for typed data channels when the event itself contains meaningful domain data.

---

## 14. Observation and Side Effects

Observers inspect published data without becoming participants in the primary computational routing graph.

This layer exists for side effects.

Examples include:

- UI publishing;
- telemetry;
- diagnostics;
- audit logging;
- capture or replay-frame storage;
- external instrumentation.

Conceptually:

```text
                    published value
                         │
              ┌──────────┴──────────┐
              ▼                     ▼
       subscriptions             observers
              │                     │
       domain computation       side effects
```

An observer MUST NOT determine whether normal subscribers receive a value.

An observer SHOULD NOT contain domain computation required for correctness of downstream analytical stages.

If removing an observer changes the mathematical result of the pipeline, that observer is probably a subscriber instead.

Observers SHOULD remain sufficiently lightweight that observational side effects do not dominate the hot publishing path.

---

## 15. Data Plane and Side-Effect Plane

The Workspace distinguishes two broad flows.

### 15.1 Data plane

The data plane carries values whose processing produces further domain state.

Examples:

```text
tickers
trades
level3
measurements
categories
resonance
causal outputs
cognition
graphs
decisions
executions
```

These use typed channels and subscriptions.

### 15.2 Side-effect plane

The side-effect plane observes what occurred and exposes or records it elsewhere.

Examples:

```text
dashboard projection
telemetry
audit
diagnostics
capture
```

These SHOULD use observers when they do not participate in domain computation.

The side-effect plane MUST NOT become an implicit prerequisite of the analytical data plane.

---

## 16. Failure Contract

A subscription step may fail.

A failure in a required processing stage MUST NOT disappear silently while the rest of the pipeline continues under the assumption that the stage succeeded.

The Workspace therefore preserves the first stage failure and propagates it to the configured runtime failure handler.

A fatal stage failure SHOULD cancel further Workspace processing.

The error path MUST preserve enough context to identify the channel or stage responsible.

Observation failures and optional side-effect failures MAY have a different policy when explicitly specified, because observational infrastructure is not necessarily part of the correctness boundary.

---

## 17. Quiescence

The Workspace exposes a quiescent state for replay, testing, and other deterministic settlement boundaries.

A Workspace is idle when:

```text
no retained channel work is pending
AND
no subscription step is currently executing
```

`WaitForQuiescence` establishes the boundary:

```text
inject observation
       │
       ▼
derived work fans through Workspace
       │
       ▼
wait until no runnable/active work remains
       │
       ▼
stable settlement point
```

Quiescence means that the streaming Workspace has drained.

It does not necessarily mean that every independently owned background system in the process has completed unrelated work.

---

## 18. Diagnostics

Scheduling behavior is part of the observable runtime state.

The Workspace SHOULD make it possible to inspect:

```text
lane count
active drains
pending values
retention capacity
high-water mark
submitted values
completed values
dropped values
stage execution duration
```

These quantities describe runtime pressure.

They MUST NOT be interpreted as market measurements.

A dropped-value count indicates scheduling or consumer pressure, not market significance.

---

## 19. Explicit Non-Claims

The Workspace does not:

- interpret market data;
- produce Measurements;
- decide strategy;
- classify regimes;
- provide durable storage;
- guarantee that every streamed value survives overload;
- assign one goroutine to every channel;
- assign one goroutine to every symbol;
- assign one goroutine to every subscription;
- make shared objects immutable;
- make arbitrary component state thread-safe;
- turn observer callbacks into part of the domain dependency graph.

The Workspace guarantees execution properties only within the contracts it owns.

For example, per-key sequential subscription execution does not make an object safe when unrelated goroutines mutate that same object outside the Workspace.

---

## 20. Component Contract

A component connected to the Workspace SHOULD have a narrow computational interface.

Conceptually:

```go
func Step(value T) error
```

or:

```go
func Process(value In) (Out, bool, error)
```

The component SHOULD NOT need internal worker management merely to consume a Workspace stream.

A keyed processor MAY retain mutable state per key under the Workspace's sequential-key execution guarantee.

A component that creates independent asynchronous work outside the Workspace owns the synchronization and lifecycle of that work itself.

---

## 21. Channel Declaration Contract

A named channel is a system-level contract.

For every channel, the architecture SHOULD define exactly one:

```text
name
Go value type
keying rule
semantic payload
```

Example:

| Channel | Payload | Key |
|---|---|---|
| `tickers` | ticker observation | symbol |
| `trades` | trade observation | symbol |
| `level3` | book update | symbol |
| `measurements` | measurement | symbol |
| `ui` | UI frame | global |
| `fluid` | fluid frame | channel or global domain |

Callers MUST NOT redefine the affinity semantics of an existing channel locally.

The key determines the concurrency boundary and is therefore part of the channel's contract, not an incidental implementation parameter.

---

## 22. Shared-State Contract

A shared object SHOULD have:

```text
stable name
optional identity components
documented owner
documented mutation rules
documented reader expectations
```

The Workspace owns discovery of the object.

It does not automatically own the object's internal synchronization.

If an object is concurrently mutable, its owner MUST define the concurrency contract governing that mutation.

Shared state SHOULD be used when consumers require the current authoritative object.

Channels SHOULD be used when consumers require the sequence of changes.

---

## 23. Signaling Contract

A trigger topic SHOULD describe an event, not an object.

Prefer:

```text
disconnect
book_invalidated
configuration_changed
```

over:

```text
book
ticker
measurement
```

when the latter actually represent data.

A listener receives notification that an event occurred. If the listener needs current state, it MAY subsequently retrieve that state from the shared-object pool or another authoritative source.

This allows:

```text
Notify
   +
Shared
```

to express:

```text
the shared state changed; inspect its current value
```

without duplicating the shared object into the signaling layer.

---

## 24. Conformance Checklist

An implementation conforms to the Workspace contract only if all answers are yes.

1. Can a component consume data without owning a dedicated goroutine?
2. Does one published value fan out independently to all subscriptions?
3. Does each subscription/key combination have independent buffering?
4. Is execution sequential for one key?
5. Can independent keys execute concurrently?
6. Can an idle lane exist without owning a worker goroutine?
7. Does repeated input to an already-active lane avoid scheduling one task per value?
8. Is lane retention bounded?
9. Is overload visible through drop/pressure diagnostics?
10. Can shared runtime objects be accessed without consumers depending directly on their producers?
11. Can components react to lightweight events without abusing the typed data stream?
12. Can side effects observe published values without becoming computational dependencies?
13. Does a required stage failure propagate to the runtime failure boundary?
14. Can replay or tests wait for the streaming graph to become quiescent?
15. Is a named channel's type and keying rule consistent throughout the system?
16. Are domain interpretation and trading logic absent from the Workspace?

If any answer is no, the implementation is not conformant with this contract.

---

## 25. Files

| File | Responsibility |
|---|---|
| `workspace.go` | Workspace, typed channels, subscriptions, shared objects, signaling, observation, failures, diagnostics, and quiescence. |
| `pool.go` | Shared elastic worker execution pool and keyed shard affinity. |
| `ring.go` | Bounded per-lane retained stream with overwrite-oldest overload behavior. |
| `node.go` | Processor contracts that keep computation independent of scheduling. |
| `stream.go` | Declarative connection of processors to named Workspace topics. |
| `disruptor.go` | Specialized bounded ring infrastructure where required. |
| `splitmix.go` | Runtime pseudo-random utility used by scheduling infrastructure. |

---

## 26. Summary

The Workspace is not a collection of queues.

It is the runtime boundary between **data flow** and **execution mechanics**.

Its primary contract is:

```text
data arrives
    ↓
Workspace routes it
    ↓
only runnable lanes consume execution capacity
    ↓
components process sequentially within their key
    ↓
idle lanes release execution ownership
```

Around that core, the Workspace supplies:

```text
shared state     → Share / Shared
reactivity       → On / Notify
side effects     → Observe
failure boundary → Error / failure handler
settlement       → WaitForQuiescence
diagnostics      → channel pressure and execution timing
```

The intended result is a system in which concurrency is centralized, observable, bounded, and largely invisible to the components performing the actual computation.
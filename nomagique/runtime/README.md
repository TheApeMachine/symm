# Runtime

`runtime` composes streaming processing rings from one recursive unit: `Node`.

```go
type Node[T any] interface {
    Step(T) T
}
```

A `Workload[T]` owns one LMAX Disruptor ring. Its constructor accepts ordered
handler groups expressed as `[][]Node[T]`:

```go
signals := runtime.NewWorkload(ctx, "signals", [][]runtime.Node[*Envelope]{
    {signalA, signalB, signalC},
    {measurementLogic},
})
```

Nodes in one inner slice run concurrently. The next inner slice cannot advance
until every node in the preceding slice has completed. These are the native
LMAX handler-group barriers; runtime does not add a worker pool or secondary
scheduler.

## Workloads are Nodes

`Workload[T]` implements `Node[T]`. A Workspace can therefore compose complete
rings using exactly the same stage grammar:

```go
workspace := runtime.NewWorkspace(ctx, "workspace", [][]runtime.Node[*Envelope]{
    {tickerWorkload, tradeWorkload, level3Workload, futuresWorkload},
    {logicWorkload},
    {strategyWorkload},
})
```

This is a ring of rings.

Every Workspace stage uses the same handler-group semantics. Submit each
observation to `workspace.Push`; all Workloads in the first stage receive it.
Their final internal stages complete before the next Workspace stage advances.
There is no special ingress stage or child-to-parent forwarding path.

```text
                       ┌─ [ signals → resonance ] ─┐
workspace.Push ────────┤                           ├─ [ classification ] ─ [ grid ] ─ [ agents ]
                       └─ [ CVD + Hawkes → manifold ]┘
```

Category currently reads the same envelope's signal measurements, so the
application places category/cognition after the signal/flow join. It cannot
run concurrently with the producers whose outputs it consumes.

## Composition is reported, not inferred

The name given to a ring is not decoration. While `Workload` builds its stages
it calls `Compose(group, stage)` on every node that implements `Composed`,
handing over the ring it belongs to and the handler group it sits behind:

```go
type Composed interface {
    Compose(group string, stage int)
}
```

This is the only place both facts are known. Nodes in one handler group run
concurrently against the same value, so anything they emit downstream is
ordered by goroutine completion — a consumer reading that output cannot tell a
real hop from two siblings racing to report. `system.Diagnostic` implements
`Composed` for exactly this reason, and the diagnostics surface draws its
groups from what the rings report rather than from a naming convention.

## Completion contract

`Workload.Step(value)` commits `value` to the Workload ring and returns only
after its final handler group has completed that sequence. Consequently, when
an outer LMAX handler calls a Workload as a Node, the outer handler does not
report completion early. The outer barrier therefore represents real nested
completion.

`Workload.Push(value)` is the asynchronous ingress operation. It commits the
event and returns so source readers are governed by the ring's native
backpressure.

Both operations use the Disruptor's reservation and sequence barriers. The
runtime does not use mutexes, per-event channels, sleeps, arbitrary polling
durations, or queues beside the declared rings.

## Ownership across rings

A stateful Node belongs to one Workload. LMAX intentionally pipelines ring
sequences: an upstream handler may advance event N+1 while a downstream handler
still processes event N. The completion barrier protects ring slots and stage
ordering; it does not make a pointer to upstream mutable state immutable.

Producer-owned mutable state must therefore be consumed synchronously inside
the same Node call that advances it, or converted to an immutable result before
the call returns:

```text
advance state -> synchronous observer -> discard live pointer -> return
```

Such a reference must never cross a handler-group or Workload boundary. Any
asynchronous consumer receives already serialized or otherwise immutable data.
This preserves pipeline overlap without locks or per-event model clones.

## Admission

New Workloads and Workspaces begin in `WAITING`. `Workspace.Admit()` opens the
nested rings and then the outer ring only after the complete subscription universe has been
constructed. Pushes before admission are rejected. This keeps partial startup
streams from becoming trading input.

## Shutdown

Shutdown stops new publication to the outer ring, waits for active publishers,
drains its listeners, and then closes its children recursively. A committed
nested Step waits for completion even if its context is cancelled. The ring
slots are cleared after their final handlers finish, releasing envelope references.

## Backlog

A Node may additionally implement `BacklogStepper[T]`. Its backlog is the
difference between the Workload producer sequence and the sequence currently
being handled. This is actual ring pressure, not a rate estimate.

## Configuration

Ring capacity comes from `runtime.workspace.buffer`. It must be a power of two;
the corresponding mask is derived from that capacity. Each ring uses the
library's shared sequencer (`WriterCount(2)` selects its multi-writer mode),
so the public ingress API supports concurrent producers. Reservation under
backpressure uses the library's `TryReserve`; cancellation can interrupt a
producer before reservation. Every successful reservation is committed.

`Step` waits with `runtime.Gosched` for the inner completion sequence. This
synchronous per-observation handoff limits how far an outer consumer can feed
an inner ring ahead. Benchmarks report that cost; this is not a claim that
nested rings are faster than a single ring.

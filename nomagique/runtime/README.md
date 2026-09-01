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

The first Workspace stage declares independently pushed ingress Workloads.
Each owns its input ring and processes only its own source stream. Its final
handler calls the outer Workspace as a Node and remains inside that call until
the complete downstream path finishes. The remaining Workspace stages are the
outer ring's handler groups. They may contain ordinary Nodes or more Workload
rings.

```text
ticker.Push ──> [ ticker workload ring ] ─┐
trade.Push  ──> [ trade workload ring  ] ─┼─> [ logic ring ] ─> [ strategy ring ]
level3.Push ──> [ level3 workload ring ] ─┘
```

There is no public attachment protocol and no runtime type router. Passing the
Workloads to `NewWorkspace` is the topology declaration. Workspace wires the
declared ingress completions before admission.

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
outer and nested rings only after the complete subscription universe has been
constructed. Pushes before admission are rejected. This keeps partial startup
streams from becoming trading input.

## Shutdown

Shutdown closes and drains ingress rings first, then drains the outer Workspace
ring, then closes nested downstream Workloads. This preserves every committed
event's declared route while preventing new ingress.

## Backlog

A Node may additionally implement `BacklogStepper[T]`. Its backlog is the
difference between the Workload producer sequence and the sequence currently
being handled. This is actual ring pressure, not a rate estimate.

## Configuration

Ring capacity comes from `runtime.workspace.buffer`. It must be a power of two;
the corresponding mask is derived from that capacity. The outer ring's writer
count is derived from the number of independently pushed ingress Workloads in
the first Workspace stage. Nested Workloads have one outer handler as their
writer.

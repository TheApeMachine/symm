# `signal/compute` — not a signal

> Two solvers reaching for the same GPU at once doesn't average out. It
> crashes.

## What this package is

Unlike every other package under `signal/`, `compute` does not measure
anything about the market. It is a small concurrency utility that the GPU-
backed solvers (physics/manifold, learning/resonance) share to avoid a real
hardware failure mode: **concurrent first-use Metal device, library, and
pipeline creation on Apple Silicon can exceed the AGX compiled-variant
footprint limit** — if two solvers both do their first-ever Metal
initialization at the same moment, the driver can fail outright rather than
just being slow.

## What it does

`WithMetalInit(init func() error) error` is a single global mutex
implemented as a spin-lock over an `atomic.Uint32` gate
(`metalInitGate`), so that whichever solver performs Metal setup first blocks
every other solver's Metal setup until it finishes — serializing the one
narrow window (device/library/pipeline creation) that is unsafe to run
concurrently, without imposing any ordering on the solvers' actual
steady-state GPU work afterward.

```go
for !metalInitGate.CompareAndSwap(0, 1) {
    runtime.Gosched()
}
defer metalInitGate.Store(0)
return init()
```

`runtime.Gosched()` rather than a channel or `sync.Mutex` is a deliberate
choice for a gate that is expected to be held only briefly and rarely
contended after startup — the spin avoids the allocation and scheduling
overhead of a full mutex for what is, after the first few ticks of process
lifetime, an uncontended fast path.

## Files

| File | Responsibility |
|---|---|
| `metal_init.go` | `WithMetalInit` — the serialization gate itself. |

There are no metrics, no measurements, and no `Signal` type in this package —
it is infrastructure the actual GPU-backed signals depend on, not a signal in
its own right.

# nomagique

`nomagique` is a Go package implementing a universal numeric reducer engine built from interned `Frame` slots, pure `Primitive` transitions, and explicit stream ownership.

The core idea is a small, allocation-friendly data model where all numeric values live in a fixed-size, value-type `Frame`. Reducers are pure functions (`Primitive`) that take an immutable state snapshot and an input `Frame` and return a next state and output `Frame`. Composition primitives like `Pipe` and `Fork` allow building complex pipelines without shared mutable maps.

## Core concepts

- **Frame**: The universal numeric payload and state representation. Values occupy interned symbol offsets in contiguous memory, while a bit mask records which slots are present. `Frame` is a value type; reducers receive snapshots by value and return committed snapshots.
- **Symbol**: Interned identifiers for slots. Created via `Intern`/`MustIntern`.
- **Primitive**: `func(state Frame, input Frame) (nextState Frame, output Frame, err error)`. The universal reducer contract.
- **Streams**: `Stream` and `KeyedStreams` provide ownership and per-key state isolation for primitives.
- **Named / Number**: Helpers for typed access to frames.

## Repository layout

Current contents of `github.com/theapemachine/symm/nomagique`:

### Root files
- `doc.go` – package documentation
- `error.go` – error helpers
- `frame.go`, `frame_test.go` – `Frame` type, `Get`/`Put`/`MustGet`, merge semantics
- `primitive.go` – `Primitive` type, `Step`, `Pipe`, `Fork`
- `stream.go`, `stream_test.go` – stream execution
- `keyed_stream.go`, `keyed_stream_test.go` – keyed stream execution
- `symbol.go` – symbol interning
- `named.go`, `named_test.go` – named access helpers
- `number.go` – numeric helpers
- `samples.go` – sample utilities
- `example_test.go` – usage examples

### Sub-packages

- **algo/**
  - `hawkes.go`, `hawkes_test.go` – Hawkes process primitives
  - `ignition.go`, `ignition.md`, `ignition_history.go`, `ignition_score.go`, `ignition_test.go` – ignition detection/scoring

- **calculus/**
  - `arithmetic.go`, `helpers.go`, `nonlinear.go`, `stateful.go`, `symbols.go`, `calculus_test.go` – basic arithmetic, nonlinear ops, stateful reducers and shared symbols

- **logic/**
  - `gate.go`, `gate_test.go` – gating primitives

- **probability/**
  - `geomean.go`, `geomean_test.go` – geometric mean reducer

- **statistic/**
  - `branching.go`, `likelihood.go`, `maximum.go`, `median.go`, `samples.go`, `statistic_test.go` – statistical reducers

- **temporal/**
  - `clock.go`, `duration.go`, `interval.go`, `temporal_test.go` – time-related primitives

- **transport/**
  - `generator.go`, `ring.go`, `ring_test.go`, `window.go`, `window_test.go` – stream transport, ring buffers and sliding windows

- **types/**
  - `descriptor.go`, `doc.go`, `frame.go`, `group.go`, `measurement.go`, `metric.go`, `pair.go`, `timescale.go`, `unit.go`, `types_test.go` – type descriptors for metrics, measurements, units, etc.

## Example

```go
import (
    "github.com/theapemachine/symm/nomagique"
    "github.com/theapemachine/symm/nomagique/calculus"
)

input := nomagique.Frame{}
input.Put(calculus.SymbolLeft, 3)
input.Put(calculus.SymbolRight, 4)
_, output, err := nomagique.Step(calculus.Sum, nomagique.Frame{}, input)
// output.MustGet(calculus.SymbolResult) == 7
```

## License
Part of the `symm` monorepo. See repository root for licensing details.

# nomagique

> **nomagique.Number**  
> *no, magic, number*

`nomagique` is a pure streaming composable algebra for high-throughput, real-time calculation, computation and machine learning.

Every component in `nomagique` conforms to a single universal transformation contract. There are no arbitrary thresholds, no hardcoded polling intervals, no magic multipliers, and no procedural glue code. Every value is derived; every transformation is an atomic primitive; every system is a composition.

---

## 1. What nomagique Is

At its core, `nomagique` is built on some foundational pillars:

1. **A Composable Functional Algebra**: Everything operates on `types.Value[T, U]` closures (`func(T) U`). Primitives connect end-to-end like mathematical operators. The output of one primitive directly feeds the input of the next without intermediate memory allocations or boxing.
2. **No Magic Numbers**: Belief, statistical baselines, horizons, confidence intervals, and regime shifts must be derived from honest data and measured uncertainty—never hardcoded constants, heuristic thresholds, or fake fallback defaults.
3. **Radical Minimalism**: Every primitive performs exactly **one** mathematical, structural, or transport transformation, and does it completely.
4. **No Domain Leakage**: This package will be extracted into a stand-alone package to be used in many projects, it is not specific to markets, finance, or crypto. Domain specific language must not leak into nomagique. It is about computation, logic, data, etc. All generically conceptualized.

### The Universal Contract

Every atom in `nomagique` is a `types.Value[T, U]` closure:

```go
type Value[T, U any] func(T) U
```

Constructors accept port closures (`types.Float`, `types.Integer`, `types.String`, etc.) enabling dynamic graphical composition via Flume and direct programmatic execution:
- Arithmetic (`arithmetic.Multiply`, `arithmetic.Divide`, `arithmetic.Add`)
- Statistics (`statistic.Mean`, `statistic.Variance`, `statistic.EMA`)
- Learning (`learning.LinearFit`, `learning.LinearPrediction`, `learning.TaskLearner`)
- Store (`tables.IcebergTable`, `tables.IcebergScan`)
- Transport (`transport.HTTPRequest`, `transport.WebSocketServer`)
- Mathematical and statistical stages are primitives (`statistic.Sympathy`, `geometry.Inversion` implement `Primitive`).

Any primitive can plug into any other primitive.

---

## 2. Universal Data Primitives (No Bespoke DTOs)

A primary failure mode in streaming engines is **DTO sprawl**—inventing bespoke structs for every stage (`PathReading`, `Observation`, `GateReading`, `Measurement`). Bespoke structs create rigid, non-composable islands that require custom adapters, helper functions, and manual dispatch loops.

`nomagique` solves this by standardizing data flow on **four universal primitives**:

### 1. `core.Input[Origin, Key, Value]`
Universal keyed data carrier and request primitive.
- **Key**: Any addressable path (e.g. `[]string{"ticker", "data", "price"}`, symbol `string`, or `*geometry.Coordinate`).
- **Value**: The borrowed payload pointer (e.g. `float64`, integer, raw bytes).
- **Origin**: The identity of the sender (`core.Identifiable[Origin]`).
- **Action**: The operation intent (`core.Action`).
- *Streaming behavior*: When stepped with nil input, it yields the configured request once. When stepped with arriving stream payloads, it dynamically binds arriving values without allocating.

### 2. `store.Query[T, U]`
The addressed interaction primitive for stores, grids, and distributed cells.
- Carries the caller's `core.Connectable[T]` and an intent (`core.Action`: `Identify`, `Read`, `Write`, `Execute`).
- Implements `core.Connectable[T]` to allow dynamic identity and transport pipe assignment across store boundaries.

### 3. `transport.Message[T]`
The universal carrier across network, process, or channel boundaries.
- Packages data addressed to a peer identity `T` with an event action.
- Delivers payloads matching the peer address without modifying the underlying data.

### 4. `store.KV[Origin, Key, Value]`
The canonical lock-free keyed state store.
- Executes `Input` requests directly against a lock-free map.
- Reads yield borrowed result pointers; writes update state and acknowledge through the stream.

---

## 3. How to Use nomagique

### 1. Pipeline Composition (`nomagique.NewNumber`)

Pipelines are composed using `nomagique.NewNumber`:

```go
pipeline := nomagique.NewNumber(
    gate,
    estimator,
    baseline,
)

// Drive the pipeline with a streaming sequence:
for out := range pipeline.Next(sequence.NewValue(100.5, 101.2, 99.8)) {
    val := *(*float64)(out)
    // ...
}
```

Because `nomagique.Number` is itself a `core.Primitive`, pipelines can be nested inside other pipelines or passed as stages to other constructors:

```go
subPipeline := nomagique.NewNumber(stageA, stageB)
mainPipeline := nomagique.NewNumber(subPipeline, stageC)
```

### 2. Driving Flexibility via Intent on the Wire

Instead of inflating a component's API with multiple procedural methods (`Set()`, `Get()`, `Execute()`, `Register()`), primitives have **exactly one method**: `Next`.

Flexibility is driven by passing intent directly through the stream:

```go
// A Write operation:
write := core.NewInput[string](nil, core.Write, "price", &priceVal)
pipeline.Next(write.Next(nil))

// A Read operation:
read := core.NewInput[string, string, float64](nil, core.Read, "price", nil)
res := sequence.Read[float64](pipeline.Next(read.Next(nil)))
```

### 3. The Transport Control Plane

Do not write imperative Go loops, goroutines, mutexes, or channels to wire components together. Use the `transport` primitives:

- **`transport.Fan`**: Splits one stream across multiple parallel branches without procedural dispatch loops.
- **`transport.Multiplex`**: Merges multiple streaming channels into a unified stream.
- **`transport.Address`**: Filters stream delivery strictly to matched identities.
- **`transport.IO`**: Connects two primitives across arbitrary boundaries or distances.
- **`transport.Once`**: Ensures a stream executes only once.
- **`transport.Discard`**: Terminal sink for unused stream branches.

#### Example: Broadcasting with `Fan`
```go
broadcaster := transport.NewFan(
    metricPipelineA,
    metricPipelineB,
    metricPipelineC,
)

// One input tick is presented to all branches automatically:
broadcaster.Next(rawMarketData)
```

### 4. Conceptual Distributed Grids (`store.Grid`)

A `Grid` in `nomagique` is not a monolithic database or centralized array; its cells remain with their owners (e.g. metrics in a signal).

1. **Declared Interests**: Each cell declares the raw data paths it consumes (`[][]string{{"ticker", "data", "price"}}`).
2. **Registration via `core.Identify`**:
   ```go
   cell := store.NewCell[*geometry.Coordinate](nil, interests, pipeline)
   query := store.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
       cell, core.Identify,
   )
   grid.Next(query.Next(sequence.NewValue(interests)))
   ```
3. **Dynamic Assignment**: The grid allocates a `*geometry.Coordinate`, calls `query.Identify(coord)` on the cell, and connects an addressable bi-directional pipe (`transport.Address` + `transport.IO`).
4. **Autonomous Streaming**: The cell processes raw data matching its interests and publishes observations across its assigned pipe back into the grid, flowing into downstream associative learning (`learning/associative/grid.go`).

---

## 4. The Non-Negotiable Rules

When working in `nomagique`, follow these rules to the letter:

### Rule 1: What Defines a Primitive
- A Primitive is a **type definition aliasing `types.Value[T, U]`** and a **constructor** returning that closure.
- A Primitive does **one** thing and does it well. It is a single mathematical, statistical, learning, or transport transformation.
- A Primitive takes typed port closures (`types.Float`, `types.Integer`, etc.) in its constructor for dynamic graph wiring.
- A Primitive has **zero ceremony**: no boilerplate structs, no wrapper methods, no unsafe pointer gymnastics.

### Rule 2: Zero Helper Functions
- Never write free helper functions (e.g. `func medianSpacing(...)`, `func quotedPrice(...)`) inside primitive files.
- When you run into a missing calculation or transformation, the answer is always: **another Primitive**.

### Rule 3: No Custom DTO Islands
- Do not create bespoke struct types for passing data between stages.
- Rely on universal numeric streams (`float64`), `core.Input`, `store.Query`, and `transport.Message`.

### Rule 4: Control Flow and Error Handling
- Guard clauses and early returns.
- **No `else` blocks.**
- Maximum two nested `if` levels.
- Every non-leading `if` must have an empty line above it.
- No single-character variable names except `t *testing.T` and `b *testing.B`.
- Returned errors pass through `errnie.Error`.
- No NaN or Inf checks, no fallback defaults, no synthetic baseline fudging.

### Rule 5: Algo and Equations are Pre-Composed Primitive Pipelines
- Composition **only** and never any implementation code.
- Equations are expressed as a pipeline of existing primitives.
- Algo is like Equation, with the added restriction that it may only contain well-known, named algorithms

## Rule 6: "Pre-Existing" Does Not Negate the Rules
- Anything that breaks the rules must be cleaned up, according to the rules.
- Rule-breaking, pre-existing code is "technical debt" and must be refactored.
- It is **not** an invitation to: start breaking the rules, use the rule-breaking pieces, ignore the rule-breaking pieces.
- Observing a rule-breaking piece of code equates to ownership, and the responsibility to resolve the technical debt.
- Resolution may **never ever under any circumstance** diminish the functionality.

---

## 5. How to Extend nomagique

Before writing a single line of new code, always evaluate in this exact order:

```
                      [Need New Functionality]
                                 │
                                 ▼
                 Does it already exist as a Primitive?
                  ├── YES ──> USE IT DIRECTLY
                  └── NO
                       │
                       ▼
          Can an existing Primitive be made more flexible?
          (e.g. expanding constructor, wrapping as decorator)
                  ├── YES ──> EXPAND CONSTRUCTOR / WRAP
                  └── NO
                       │
                       ▼
          Can it be composed from multiple existing Primitives?
          (e.g. using nomagique.NewNumber, transport.Fan, logic)
                  ├── YES ──> COMPOSE IN equation/ OR algo/
                  └── NO
                       │
                       ▼
          Implement ONE new atomic Primitive in its canonical package
```

### Package Organization
- **`nomagique/core`**: Foundational algebra interfaces, errors, actions, input abstractions.
- **`nomagique/arithmetic`**: Fundamental arithmetic transformations (`Add`, `Subtract`, `Multiply`, `Divide`).
- **`nomagique/logic`**: Conditional branching and gate primitives (`Finite`, `Gate`, `Greater`, `Equal`).
- **`nomagique/statistic`**: Derived statistical estimators (`Concordance`, `Sympathy`, `Variance`, `Quantile`).
- **`nomagique/geometry`**: Spatial mapping, clustering, and manifold primitives (`Mapping`, `Inversion`, `Relaxation`, `Forest`, `Peak`, `Border`).
- **`nomagique/transport`**: Streaming control plane (`Fan`, `Multiplex`, `IO`, `Address`, `Message`, `Once`, `Discard`).
- **`nomagique/store`**: Coordinate and keyed storage primitives (`Grid`, `KV`, `Query`, `Cell`).
- **`nomagique/equation`**: Pure compositions of existing primitives with **zero implementation code**.
- **`nomagique/algo`**: Named, well-known algorithms composed from primitives.
- **`nomagique/physics`**: **Protected** (do not modify without explicit instruction).

### Primitive Blueprint

When implementing a new primitive:

```go
package domain

import (
    "github.com/theapemachine/symm/nomagique/types"
)

/*
Transform performs a single mathematical or computational operation.
*/
type Transform types.Value[float64, float64]

func NewTransform(factor types.Float) Transform {
    return func(in float64) float64 {
        f := 1.0
        if factor != nil {
            f = factor(in)
        }
        return in * f
    }
}
```

> **Remember**: You are not implementing the end goal in `nomagique`. You are implementing the universal algebra of Primitives so that signals, solvers, and market systems can build their pipelines anywhere.

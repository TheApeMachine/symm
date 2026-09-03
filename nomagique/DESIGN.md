# nomagique v2 — System Architecture Specification

**Status:** Definitive Architecture Specification  
**Classification:** Core Runtime Engine  
**Supersedes:** `nomagique v1` (`Frame`-based slot-interning engine)  

---

> NOTE!
> Any reference or naming related to crypto, markets, or any other domain specificity is entirely accidental, and nomagique needs to omit such nameing in favor of generic math/computaiton/etc. terminology.
> The nomagique package is not specific to symm, and will eventually be extracted into a stand-alone package.

## 1. System Mission & The Zero-Constant Principle

The package is named for the foundational rule it exists to enforce:
$$\text{nomagique} = \text{no magic} \implies \text{nomagique.Number} = \textbf{no magic numbers}$$

In streaming signal processing, financial microstructure, and real-time statistics, hardcoded scalar constants—such as a 32-period moving average, a 2.5-sigma outlier cutoff, or a 10-millisecond calendar timer—are static, unproven assumptions imposed on a non-stationary world. When market volatility shifts or transaction density changes, static coefficients instantly degrade: lookback windows lag, thresholds fail to filter noise, and fixed clocks miss burst events.

**The Zero-Constant Principle dictates:**
> No primitive in `nomagique` accepts hardcoded operational constants. Every parameter that shapes, bounds, filters, or attenuates a stream must be an **emergent, adaptive dynamic derived causally from the data itself**.

Parameters are not bare floats or integers; they are typed, adaptive engines evaluated online:

```
┌────────────────────────────────────────────────────────────────────────┐
│                        THE FOUR ADAPTIVE ENGINES                       │
├──────────────────┬──────────────────┬─────────────────┬────────────────┤
│      Window      │    Threshold     │      Clock      │    Envelope    │
│    (Horizon)     │     (Filter)     │  (Attenuation)  │   (Boundary)   │
├──────────────────┼──────────────────┼─────────────────┼────────────────┤
│ • ADWIN          │ • WELFORD        │ • INTERARRIVAL  │ • EVT (Pareto) │
│ • STABILITY_GOV  │ • MAD            │ • VOLUME        │ • CHEBYSHEV    │
│ • KISH           │ • CHEBYSHEV      │ • ENTROPY       │ • BOLLINGER    │
└──────────────────┴──────────────────┴─────────────────┴────────────────┘
```

- **Adaptive Windows** expand when data is stationary and contract automatically when change or concept drift is statistically detected (e.g., ADWIN, Bifet & Gavaldà, 2007).
- **Adaptive Thresholds** self-calibrate to running dispersion and empirical kurtosis rather than fixed sigma multipliers (e.g., Welford variance, Median Absolute Deviation).
- **Adaptive Clocks** decay in information time (tick volume, arrival entropy) rather than frozen calendar milliseconds.
- **Adaptive Envelopes** establish support frontiers using Extreme Value Theory (EVT) rather than arbitrary minimum/maximum clamps.

---

## 2. Post-Mortem: Why v1 Collapsed

The original architecture of `nomagique` rested on a single premise: `type Primitive func(frame *Frame)`, where every primitive mutated a single flat, globally interned 97 KB slot array.

Almost every defect and performance failure in v1 was an unavoidable consequence of that design:

```
v1 Global Internees:
┌──────────────────────────────────────────────────────────────────┐
│ Global Symbol Table: [Slot 0] [Slot 1] ... [Slot 8191] (PANIC!)  │
└──────────────────────────────────────────────────────────────────┘
       ▲                     ▲                     ▲
       │                     │                     │
   Primitive A           Primitive B           Primitive C
 (Needs offset)        (Needs offset)        (Needs offset)
```

| Symptom in v1                                  | Root Cause in v1 Architecture                                                                       | Resolution in v2                                                                                              |
|------------------------------------------------|-----------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `panic: symbol registry is full (8192 slots)`  | Every value required a globally unique offset in an interned table.                                 | **Private State Ownership:** Nodes own their state directly in memory. The symbol registry is deleted.        |
| `Wire`/`Binding`/`In`/`Out` (~280 call sites)  | Primitives lacked encapsulation; inputs and outputs had to be marshaled via global offsets.         | **Closed Contract:** Composition *is* the binding. Inputs and outputs pass directly between parent and child. |
| `prefix string` threaded everywhere            | Manual namespacing was required to prevent slot collisions in the shared address space.             | **Object Isolation:** Nodes are isolated Go structs on the stack or heap.                                     |
| `Fork` copying 97 KB `Frame` per branch        | Branches shared one address space, requiring massive array diffs (`MergeChanged`) to isolate state. | **Isolated Dataflow:** Branches evaluate locally; no global memory is synchronized.                           |
| `MaxSamples = 128` hard-clamped adaptive loops | State lived in fixed-width struct arrays, making growable windows impossible.                       | **Unbounded Growth:** Stateful primitives hold standard growable slices, fulfilling the zero-magic mandate.   |

The most critical defect was the violation of its own name: to keep the 97 KB `Frame` from expanding, `temporal/governor.go`, `statistic/baseline.go`, and `temporal/window.go` hard-clamped adaptive loops at 128 samples. When a stream needed to adapt beyond 128 samples, it silently pinned, rendering stability signals meaningless.

**The v2 fix is not a larger global frame. It is the total elimination of the frame.**

---

## 3. The Dual Representation: Carrier & Engine

To build a high-performance streaming mathematical engine in Go, the architecture must resolve a fundamental type-system constraint:

1. **The carrier must behave as a native number** (`val += 1.0`, `val * 0.25`, passing to `math.Sqrt`).
2. **The processing engine must own growable, stateful memory** (ring buffers, EMA levels, clock timestamps).

Go does not support operator overloading. A Go `struct` can own memory, but it cannot use `+`, `-`, `*`, or `/`. A Go `float64` can use arithmetic operators, but it is strictly 8 bytes and cannot hold private slices or state without an ambient global registry.

v2 resolves this by cleanly decoupling the **Carrier** from the **Engine**.

```
   ┌────────────────────────────────────────────────────────┐
   │                  THE DUAL REPRESENTATION               │
   ├────────────────────────────┬───────────────────────────┤
   │    Carrier: Number         │    Engine: Node           │
   │    (What Flows)            │    (What Transforms)      │
   ├────────────────────────────┼───────────────────────────┤
   │ • type Number float64      │ • type Node interface     │
   │ • 8 bytes, unboxed         │ • Step(Number) Number     │
   │ • Native Go arithmetic     │ • Owns private state      │
   │ • Participates in +, -, *  │ • Zero steady-state alloc │
   └────────────────────────────┴───────────────────────────┘
```

### 3.1 The Carrier: `nomagique.Number`
The carrier is a defined float type:
```go
package nomagique

type Number float64

func (n Number) Through(node Node) Number {
    return node.Step(n)
}
```

Because its underlying type is `float64`, it participates natively in Go arithmetic without `.Value()`, unboxing, or interface wrappers:
```go
var n nomagique.Number = 100.0
n += 1.0                      // Native Go floating-point addition
scaled := n * 0.25            // Native Go floating-point multiplication
sqrt := math.Sqrt(float64(n)) // Zero-cost conversion for stdlib interop
```

### 3.2 The Engine: The Closed `Node` Contract
All transformations, filters, equations, and reductions satisfy one closed contract:
```go
type Node interface {
    Step(Number) Number
}
```

Because `Step` consumes and returns `Number`, any node—or composition of nodes—can serve as an input to any other. Composition is structural closure over `Number`.

### 3.3 The Boxing Trap
The naive implementation of a closed interface allocates an 8-byte box on the heap on every return. At high streaming frequencies, this is fatal:

```
BenchmarkClosedIface-16     206282416    5.623 ns/op    8 B/op    1 allocs/op
BenchmarkUnboxedCarrier-16   742031090    1.602 ns/op    0 B/op    0 allocs/op
```

**Design Rule:** While `Node` is the public composition interface, all inner execution paths must remain monomorphic and non-allocating. In steady state, a step through a graph must produce **0 allocations per operation**.

---

## 4. The Complete Topological Algebra

Dataflow graphs operate across two orthogonal spatial dimensions:
1. **Series (Time / Flow):** Transforming an input sequentially through successive stages ($x \to A \to B \to y$).
2. **Parallel (Space / Distribution):** Broadcasting an input concurrently to multiple branches and combining their outputs ($x \to [A(x), B(x)] \to Combine(A, B)$).

v2 formalizes this into two core structural nodes: **`Chain`** and **`Split`**.

```
         Parallel: Split                            Series: Chain
         
          ┌──> [ A ] ──┐
   x ────┤              ├──> Combine(A, B)   x ───> [ A ] ───> [ B ] ───> y
          └──> [ B ] ──┘
```

### 4.1 Series Composition: `Chain`
`Chain` executes an ordered sequence of transformations. The output of slot `A` feeds directly into slot `B`:

```go
type Chain struct {
    A, B, C, D Node
}

func (c *Chain) Step(x Number) Number {
    if c.A != nil { x = c.A.Step(x) }
    if c.B != nil { x = c.B.Step(x) }
    if c.C != nil { x = c.C.Step(x) }
    if c.D != nil { x = c.D.Step(x) }
    return x
}
```

#### The Zero-Value Rule for Series
- **Identity Element:** The functional identity wire $I(x) = x$.
- **Degenerate Behavior:** An empty slot in a `Chain` is transparent. If all slots are omitted, `Chain{}.Step(x)` returns `x` unchanged.

---

### 4.2 Parallel Composition: `Split`
`Split` distributes an incoming signal $x$ across parallel branches and computes their weighted summation:

$$\text{Output} = \sum_{i \in \{A, B, C, D\}} w_i \cdot \text{Branch}_i(x)$$

```go
type Split struct {
    Route   Router
    A, B, C, D Node
}

func (s *Split) Step(x Number) Number {
    wA, wB, wC, wD := Number(1), Number(1), Number(1), Number(1)
    if s.Route != nil {
        wA, wB, wC, wD = s.Route.Route(x)
    }

    var sum Number
    if s.A != nil && wA > 0 { sum += wA * s.A.Step(x) }
    if s.B != nil && wB > 0 { sum += wB * s.B.Step(x) }
    if s.C != nil && wC > 0 { sum += wC * s.C.Step(x) }
    if s.D != nil && wD > 0 { sum += wD * s.D.Step(x) }
    return sum
}
```

#### The Zero-Value Rule for Parallel
- **Identity Element:** The additive identity $0$.
- **Degenerate Behavior:** An empty slot in a `Split` contributes $0$ to the sum. An empty `Split{}.Step(x)` returns $0$.

---

### 4.3 The Universal Condition Key: `Route`

`Split` eliminates the need for separate `Tee`, `Gate`, `Blend`, and `Mux` primitives. All four behaviors are configurations of the single condition key: `Route`.

```go
type Router interface {
    Route(x Number) (Number, Number, Number, Number)
}
```

| Mode          | `Route` Setting        | Mathematical Meaning                                                    | Classical Name        |
|---------------|------------------------|-------------------------------------------------------------------------|-----------------------|
| **Broadcast** | `nil` (omitted)        | Weights default to $[1, 1, 1, 1]$; all branches evaluated and summed    | **Tee / Fork-Sum**    |
| **Switch**    | `Pick{Index: i}`       | Weight is $1.0$ on chosen branch $i$; all other branches $0.0$          | **Mux / Router**      |
| **Gating**    | `Threshold{Type: ...}` | Weights are $[1, 0, 0, 0]$ or $[0, 1, 0, 0]$ based on adaptive boundary | **Gate / If-Else**    |
| **Blending**  | `Mix{Type: ...}`       | Fractional weights summing to $1.0$ ($w_A + w_B = 1.0$)                 | **Blend / Crossfade** |

---

### 4.4 The Law of Sinks (How `Tee` Emerges Naturally)

In v1, tapping a stream to record samples without modifying the calculation required complex plumbing. In v2, a `Tee` requires no specialized primitive; it emerges from an unconditioned `Split` paired with an **algebraic Sink**.

A sink is an observational primitive (such as `Store`) whose sole responsibility is state capture. To avoid mutating the downstream carrier:
> **An algebraic Sink returns $0$ from its `Step` method.**

When placed inside an unconditioned `Split`:
$$\text{Output} = 1.0 \cdot \text{Compute}(x) + 1.0 \cdot \text{Store}(x) = \text{Compute}(x) + 0 = \text{Compute}(x)$$

```
                         ┌──> [ Compute(x) ] ──> Result ──┐
                  x ─────┤                                ├──> Result + 0 = Result
                         └──> [ &Store{} ]   ──>   0    ──┘
                              (records x)
```

The sample is captured into memory, the main calculation passes through uncorrupted, and no special wrapper is required.

---

### 4.5 Arity: Fixed $A, B, C, D$ vs. Slices

`Chain` and `Split` enforce a fixed 4-slot layout (`A, B, C, D`) rather than variadic slices (`[]Node`):

1. **Zero Heap Allocations:** Fixed struct fields live inline on the stack or inside the parent allocation. A slice header requires an immediate heap allocation for its backing array during literal initialization.
2. **L1 Cache Alignment:** An 8-pointer/interface struct fits neatly within standard 64-byte processor cache lines.
3. **Hierarchy Over Width:** Decision trees with $>4$ paths are naturally expressed as nested hierarchical splits:
   ```go
   Split{
       A: Split{ A: node1, B: node2 },
       B: Split{ A: node3, B: node4 },
   }
   ```

---

## 5. Slot Semantics & Naming Conventions

To ensure any engineer can read an unfamiliar graph without documentation, three conventions apply across the runtime.

### C1: Topological vs. Parametric Slots
- **Topological Nodes (`Chain`, `Split`, `Sum`, `Product`):** Slots use positional identifiers `A, B, C, D`. These are symmetric structural channels.
- **Parametric Primitives (`Decay`, `Standardize`):** Slots use **semantic names** (`Rate`, `Shape`, `Center`, `Scale`). Positional anonymous slots are prohibited here because swapping asymmetric roles (such as Center and Scale) produces catastrophic, silent mathematical errors.

### C2: `Type` Exclusively Names Enumerated Strategies
The field name `Type` is reserved exclusively to choose among mutually exclusive strategies on a component, never for general configuration:
```go
Store{Type: DynamicRing}
adaptive.Window{Type: ADWIN}
adaptive.Threshold{Type: WELFORD}
adaptive.Clock{Type: INTERARRIVAL}
```

### C3: `Route` is the Universal Branching Key
Every routing, switching, or mixing decision is configured via the `Route` field of a `Split` node.

---

### 5.1 The Degenerate Zero-Value Matrix

Every primitive in `nomagique` must define what an empty slot degenerates to, proven by an automated equivalence test. An empty slot must yield the **identity element** of that operation:

| Node          | Omitted Field    | Degenerate Behavior      | Mathematical Rationale                               |
|---------------|------------------|--------------------------|------------------------------------------------------|
| `Sum`         | Slot omitted     | Contributes `0`          | Additive identity ($x + 0 = x$)                      |
| `Product`     | Slot omitted     | Contributes `1`          | Multiplicative identity ($x \cdot 1 = x$)            |
| `Chain`       | Slot omitted     | Pass-through ($x \to x$) | Functional identity ($I(x) = x$)                     |
| `Split`       | Slot omitted     | Contributes `0`          | Branch isolation                                     |
| `Decay`       | `Rate` omitted   | Instant drop to `0`      | Absence of clock implies elapsed time $t \to \infty$ |
| `Decay`       | `Shape` omitted  | Linear decay             | Absence of non-linear transfer function              |
| `Standardize` | `Center` omitted | Subtracts `0`            | Location defaults to zero                            |
| `Standardize` | `Scale` omitted  | Divides by `1`           | Dispersion defaults to unity                         |

---

## 6. The Five-Tier Taxonomy

In v1, everything was forced into a single type: `func(*Frame)`. v2 stratifies all logic into five clean layers based strictly on **cross-tick memory retention**:

```
Operation / Reduction  ──>  Primitive  ──>  Equation  ──>  Algo
   (Pure Functions)         (Stateful)      (Composed)    (Literature)
```

```
┌────────────────────────────────────────────────────────────────────────┐
│                        THE FIVE-TIER TAXONOMY                          │
├────────────┬────────────────────────┬─────────────┬────────────────────┤
│ Tier       │ Memory Across Ticks    │ Arity       │ Examples           │
├────────────┼────────────────────────┼─────────────┼────────────────────┤
│ Operation  │ None (Pure function)   │ Fixed (1–2) │ Sum, Exp, Abs      │
│ Reduction  │ None (Pure fold)       │ Collection  │ Average, Median    │
│ Primitive  │ Owns private state     │ 1 + slots   │ Store, Decay       │
│ Equation   │ Owns child primitives  │ Composed    │ Standardizer, MACD │
│ Algo       │ Manages complex state  │ Canonical   │ Hawkes, Welford    │
└────────────┴────────────────────────┴─────────────┴────────────────────┘
```

1. **Operations (Pure Arithmetic):** Completely stateless functions over `~float64`. Can be inlined or constant-folded by the Go compiler; trivially concurrent-safe.
2. **Reductions (Pure Folds):** Pure aggregations over slices (`func([]Number) Number`). They do not retain buffers; they fold buffers passed to them.
3. **Primitives (State Owners):** Nodes that retain state across ticks and expose parametric slots. This is the lowest layer where memory lives.
4. **Equations (Preset Compositions):** Structural compositions of Operations, Reductions, and Primitives. **An Equation must contain zero custom arithmetic.** If an equation requires arithmetic not provided by a lower tier, a primitive or operation is missing.
5. **Algos (Canonical Literature):** Preset equations reserved strictly for established, published algorithms (e.g., Welford 1962, Hawkes 1971, Bifet & Gavaldà 2007).

---

## 7. Performance Architecture & Memory Model

To support institutional high-frequency data pipelines, `nomagique v2` targets $<30$ nanosecond step latencies and zero steady-state heap allocations.

### 7.1 Multi-Stream Benchmark Evaluation
To determine the optimal structural representation, three approaches were benchmarked under identical processing loads across 2,200 concurrent keyed streams:

| Architectural Approach                    | Step Latency (Single) | Step Latency (2,200 Streams) | Heap Objects | GC Pause Time | Construction Cost (2,200 Streams) |
|-------------------------------------------|-----------------------|------------------------------|--------------|---------------|-----------------------------------|
| **A. Pointer Nodes + Dynamic Interface**  | 23.7 ns               | 27.7 ns                      | 13,197       | 248 µs        | 493 KB / 13,200 allocs            |
| **B. Shared Spec + Arena Slot Indexing**  | 25.7 ns               | 29.0 ns                      | 6,596        | 181 µs        | 246 KB / 8,800 allocs             |
| **C. Monomorphic Embedded Value Structs** | **19.2 ns**           | **20.3 ns**                  | **2,191**    | **283 µs**    | **268 KB / 2,200 allocs**         |

```
Step Latency Across 2,200 Streams (Lower is better)
Option C [Value Structs]   ████████████████████ 20.3 ns
Option A [Pointer Nodes]   ███████████████████████████ 27.7 ns
Option B [Arena Indexing]  █████████████████████████████ 29.0 ns
```

### 7.2 Architectural Conclusions
1. **Option B (The Arena Fallacy) is Rejected:** Decoupling specs from memory arenas halved heap object counts but added indirection overhead, yielded slower step times, and re-introduced v1's memory-pool complexity.
2. **Option C (Embedded Structs) Wins on Throughput:** Embedding child primitives *by value* directly inside concrete Equation structs allows the Go compiler to inline method calls, eliminate pointer indirection, and bypass interface dispatch tables entirely.
3. **The Dual Authoring Rule:**
   - **Authoring Layer (Ad-hoc Exploration):** Dynamic pointer trees (`&Decay{...}`) inside `nomagique.Number(...)`. This pays a minor one-time setup allocation cost while maintaining 0 allocs in steady state.
   - **Production Layer (Named Equations & Algos):** Hand-written or code-generated concrete structs embedding their children by value for maximum compiler optimization.

---

## 8. Multi-Output Resolution

Functional dataflow graphs struggle with the **Diamond Problem**: how can an upstream node produce multiple values (e.g., an online standardizer computing both Mean and Variance) and supply both to downstream consumers without breaking the single unary carrier?

```
                        ┌──> Center (Mean) ──────┐
Input ──> WelfordEngine ┤                        ├──> Standardize
                        └──> Dispersion (StdDev) ─┘
```

In a nested tree literal, referencing an intermediate output requires an intermediate variable, turning a tree into a DAG.

v2 resolves this through **Equation Encapsulation**:

Multi-output operations belong inside concrete **Equation structs**. The hot-path carrier remains `Number float64`, while auxiliary metrics are exposed via zero-cost field/method getters:

```go
type Standardizer struct {
    welford adaptive.WelfordEngine // Embedded by value (0 allocs)
    lastZ   Number
}

func (s *Standardizer) Step(x Number) Number {
    mean, stdDev := s.welford.Update(x)
    if stdDev == 0 {
        s.lastZ = 0
        return 0
    }
    s.lastZ = (x - mean) / stdDev
    return s.lastZ
}

// Auxiliary readings are zero-cost, inlined field accesses:
func (s *Standardizer) Mean() Number       { return s.welford.Mean }
func (s *Standardizer) Dispersion() Number { return s.welford.StdDev }
```

Downstream consumers in a pipeline receive the primary signal (`ZScore`), while telemetry, logging harnesses, or parallel equations read `.Mean()` and `.Dispersion()` directly from the struct handle.

---

## 9. Production Reference Implementations

Every example below demonstrates the target developer experience: **a complete, zero-allocation, adaptive signal pipeline defined in a single, nested `nomagique.Number(...)` expression without any magic constants.**

---

### Scenario 1: Information-Time Decay with Zero-Sink Buffer

**Goal:** Attenuate trade price shocks across high-frequency and structural horizons. Decay rates do not use fixed calendar milliseconds; they adapt to the inter-arrival velocity and volume of trades. Simultaneously, raw ticks are captured into a self-sizing ADWIN buffer via an algebraic zero-sink.

```go
package main

import (
	"fmt"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/adaptive"
)

func main() {
	// A complete, single-expression pipeline with zero magic constants
	pipeline := nomagique.Number(
		nomagique.Chain{
			// STAGE 1: Parallel Fan-out / Join
			A: nomagique.Split{
				// Route is omitted => Broadcast (Tee / Sum)

				// Branch A: High-frequency information decay
				A: &nomagique.Decay{
					Rate: &adaptive.Clock{
						Type:        adaptive.INTERARRIVAL,
						Sensitivity: adaptive.Sensitivity{Type: adaptive.HIGH},
					},
					Shape: nomagique.Exponential{},
				},

				// Branch B: Macro structural volume decay
				B: &nomagique.Decay{
					Rate: &adaptive.Clock{
						Type:        adaptive.VOLUME,
						Sensitivity: adaptive.Sensitivity{Type: adaptive.LOW},
					},
					Shape: nomagique.Exponential{},
				},

				// Branch C: Passive sidecar buffer (Sink)
				// Emits 0 -> Sum is uncorrupted: (A + B + 0 = A + B)
				C: &nomagique.Store{
					Type:     nomagique.DynamicRing,
					Adaptive: adaptive.Window{Type: adaptive.ADWIN},
				},
			},

			// STAGE 2: Adaptive Support Envelope (Extreme Value Theory)
			// No arbitrary [min, max] clamps; bounds adapt to distribution tails
			B: &adaptive.Envelope{Type: adaptive.EVT},
		},
	)

	// Step returns nomagique.Number (a native float64)
	out := pipeline.Step(104.5)
	fmt.Printf("Initial Step: %f\n", out)

	// Participates natively in language math with zero unboxing
	out += 1.0
	scaled := out * 0.25
	fmt.Printf("Native Math Applied: %f\n", scaled)
}
```

---

### Scenario 2: Dynamic Regime-Switching Volatility Filter

**Goal:** Construct an adaptive noise filter. The sampling horizon dynamically expands and contracts via `ADWIN`. When market volatility is calm, the filter passes through a reactive moving average; when online variance exceeds an adaptive `WELFORD` threshold, it smoothly crossfades into a robust median filter to reject outlier spikes.

```go
package main

import (
	"fmt"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/adaptive"
)

func main() {
	pipeline := nomagique.Number(
		nomagique.Split{
			// Crossfade weights emerge dynamically from data stream statistics
			Route: &nomagique.VolatilityBlend{
				Window:    adaptive.Window{Type: adaptive.ADWIN},
				Threshold: adaptive.Threshold{Type: adaptive.WELFORD},
			},

			// Branch A: Reactive Moving Average (Laminar / Calm regime)
			A: nomagique.Chain{
				A: &nomagique.Store{
					Type:     nomagique.DynamicRing,
					Adaptive: adaptive.Window{Type: adaptive.ADWIN},
				},
				B: nomagique.Average,
			},

			// Branch B: Robust Median Filter (Turbulent / Shock regime)
			B: nomagique.Chain{
				A: &nomagique.Store{
					Type:     nomagique.DynamicRing,
					Adaptive: adaptive.Window{Type: adaptive.STABILITY_GOV},
				},
				B: nomagique.Median,
			},
		},
	)

	prices := []nomagique.Number{101.1, 101.2, 101.4, 109.0, 101.5}
	for _, p := range prices {
		filtered := pipeline.Step(p)
		fmt.Printf("Input: %5.2f -> Filtered: %5.2f\n", p, filtered)
	}
}
```

---

### Scenario 3: Causal Standardizer with Chebyshev Gating

**Goal:** Ingest unnormalized order book volume, calculate online running mean and variance using Welford's algorithm without lookback bias, and scrub outliers using an adaptive, non-parametric Chebyshev confidence band.

```go
package main

import (
	"fmt"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/adaptive"
)

func main() {
	standardizer := &nomagique.Standardizer{
		Engine: &adaptive.WelfordEngine{},
	}

	pipeline := nomagique.Number(
		nomagique.Chain{
			// Stage 1: Causal online standardization
			A: standardizer,

			// Stage 2: Distribution-free dynamic outlier scrubbing
			// Replaces static clamp bounds with an adaptive Chebyshev frontier
			B: &adaptive.Gating{
				Threshold: adaptive.Threshold{Type: adaptive.CHEBYSHEV},
			},
		},
	)

	ticks := []nomagique.Number{10.0, 10.2, 10.1, 95.0, 10.3}
	for _, tick := range ticks {
		z := pipeline.Step(tick)

		// Primary output is clamped z-score; auxiliary metrics inspected on demand
		fmt.Printf("Raw: %5.1f | Z-Score: %5.2f | Causal Mean: %5.2f\n",
			tick, z, standardizer.Mean())
	}
}
```

---

### Scenario 4: Self-Exciting Hawkes Surge Tracker

**Goal:** Estimate trade arrival clustering via a Hawkes process:
$$\lambda(t) = \mu(t) + \sum_{t_i < t} \alpha \cdot e^{-\beta(t - t_i)}$$
The baseline intensity $\mu(t)$ is an emergent causal baseline, and the self-excitation decay rate $\beta$ adapts dynamically to trade entropy.

```go
package main

import (
	"fmt"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/adaptive"
)

func main() {
	hawkes := nomagique.Number(
		nomagique.Chain{
			// Parallel: Decaying self-excitation tail + raw arrival impulse
			A: nomagique.Split{
				// Branch A: Cluster excitation tail
				A: &nomagique.Decay{
					Rate:  &adaptive.Clock{Type: adaptive.ENTROPY},
					Shape: nomagique.Exponential{},
				},
				// Branch B: Raw event shock pass-through
				B: nomagique.Identity{},
			},

			// Emergent background intensity:
			// Baseline mu is an online causal baseline, not a magic scalar constant
			B: &adaptive.Baseline{
				Engine: adaptive.WelfordEngine{},
			},
		},
	)

	fmt.Printf("Resting Intensity: %f\n", hawkes.Step(0.0))
	fmt.Printf("Shock Intensity:   %f\n", hawkes.Step(5.0))
}
```

---

### Scenario 5: The Pure Adaptive Governor

**Goal:** Fulfill the foundational promise broken in v1. A window begins at zero, expands unboundedly as long as stability persists, and immediately contracts when directional entropy increases. There are no clamps (`MaxSamples = 128`), no magic thresholds (`0.15`, `0.85`), and no static lookback spans.

```go
package main

import (
	"fmt"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/adaptive"
)

func main() {
	governor := nomagique.Number(
		&nomagique.Governor{
			// Dynamically sized slice, unbounded by any global frame limit
			Store: nomagique.Store{
				Type:     nomagique.DynamicRing,
				Adaptive: adaptive.Window{Type: adaptive.ADWIN},
			},

			// Expansion and contraction driven by online information entropy
			Controller: &adaptive.StabilityController{
				Type: adaptive.KISH,
			},

			// Reduction applied over the emergent window
			Reduce: nomagique.LinearSlope,
		},
	)

	// Feed 10,000 samples — v1 clamped at 128; v2 expands smoothly to true stability
	for i := 0; i < 10000; i++ {
		slope := governor.Step(nomagique.Number(i % 100))
		if i%2000 == 0 {
			fmt.Printf("Tick %5d | Emergent Slope: %f\n", i, slope)
		}
	}
}
```

---

## 10. Migration Strategy (v1 to v2)

Migrating ~31,000 lines of production code across 36 dependent packages requires an orderly, mechanical refactoring plan.

### 10.1 Deprecation & Deletion Sequence
1. **Delete `Frame`:** Remove the 97 KB array, `MustIntern`, and all slot-registry synchronization primitives.
2. **Eliminate `Wire` Blocks:** Refactor all 282 wiring sites into direct, nested `Chain` and `Split` compositions.
3. **Remove `prefix string`:** Delete manual namespace strings from all constructors; rely on Go struct instance isolation.
4. **Remove Hard Clamps:** Uncap `temporal/governor.go:47`, `statistic/baseline.go:294,310`, and `temporal/window.go:169` (`MaxSamples = 128`).
5. **Convert Magic Numbers to Adaptive Primitives:** Replace static integer spans and float thresholds with their `adaptive.Window` and `adaptive.Threshold` equivalents.

### 10.2 Architectural Mapping Guide

| Legacy v1 Construct                 | Target v2 Construct                                                | Migration Mechanism                                                          |
|-------------------------------------|--------------------------------------------------------------------|------------------------------------------------------------------------------|
| `temporal.NewWindow(prefix, 128)`   | `Store{Type: DynamicRing, Adaptive: adaptive.Window{Type: ADWIN}}` | Replace with dynamic ring buffer; remove prefix string and magic number 128. |
| `Wire(In("a"), Out("b"), Identity)` | `Chain{ A: a, B: b }`                                              | Collapse into direct series composition.                                     |
| `logic.Gate(condition, a, b)`       | `Split{ Route: condition, A: a, B: b }`                            | Move gating logic into `Route`.                                              |
| `Tee(primary, secondary)`           | `Split{ A: primary, B: &Store{} }`                                 | Use `Split` with a zero-emitting Sink.                                       |
| `equation.CausalBaseline`           | Concrete Equation struct (Option C)                                | Embed `WelfordEngine` and `Store` by value for zero-alloc inlining.          |

---

## 11. Architectural Invariants (The System Contract)

Any pull request or extension to `nomagique` must preserve these invariants:

1. **The Zero-Constant Invariant:** No primitive shall expose a static float or integer parameter for window spans, thresholds, decay intervals, or clamp bounds. All such parameters must accept an `adaptive` engine.
2. **The Zero-Allocation Invariant:** Calling `Step(Number)` on any composed graph or equation must allocate **zero bytes on the heap** during steady-state execution.
3. **The Identity Invariant:** Every primitive must specify its degenerate zero-value behavior, and that behavior must resolve to the algebraic identity of its operation.
4. **The Unboxed Carrier Invariant:** The carrier must remain `type Number float64`. It must never be wrapped in an interface, pointer, or struct container on the hot path.
5. **The Encapsulation Invariant:** Nodes must own their state privately. No global registry, ambient symbol table, or cross-instance shared memory may exist.
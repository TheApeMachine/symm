# nomagique v2 — design

Working document. This is the architecture nomagique *should* have had. Nothing
here is implemented yet; the current package is described only where the
contrast is instructive.

Status legend: **[settled]** agreed, **[open]** needs a decision, **[proto]**
validated in a throwaway benchmark.

---

## 0. Why a v2 at all

The present implementation rests on one decision — `type Primitive
func(frame *Frame)`, mutating a single flat, globally-interned slot space — and
almost every structural problem in the package descends from it:

| Symptom                                                                 | Actual cause                                                                          |
|-------------------------------------------------------------------------|---------------------------------------------------------------------------------------|
| `panic: symbol registry is full (8192 slots)`                           | every value needs a unique *global* offset                                            |
| `Wire`/`Binding`/`In`/`Out`/`State` (~280 call sites)                   | primitives have no private namespace, so inputs must be marshalled in and out by hand |
| `prefix string` threaded through every stateful primitive               | manual namespacing to dodge collisions in the shared space                            |
| `Fork` copying a 97 KB `Frame` per branch, reconciled by `MergeChanged` | branches share one address space, so writes must be diffed                            |
| `MaxSamples = 128` clamping the adaptive loop                           | state lives in a fixed-width struct, so a window cannot actually grow                 |

The last one is the serious one. The package is named for the rule it breaks:
`nomagique.Number` — *no magic number*. A window is supposed to start at nothing
and grow until stable, then re-adapt if stability degrades. Today
`temporal/governor.go:47`, `statistic/baseline.go:294,310` and
`temporal/window.go:169` hard-clamp that loop at 128. Past the clamp a series
silently pins and its stability signal stops meaning anything.

The fix is not a bigger ceiling. It is removing the thing that needs a ceiling.

---

## 1. The closed contract **[settled]**

> Input, output, and computation are the same kind.

```go
type Primitive interface {
    Step(Primitive) Primitive
}
```

Because `Step` consumes and returns the *same* type, any primitive — or any
composition of primitives — is a legal input to any other. Composition is
closure over one type, not a protocol between two.

What this buys, concretely:

- **No symbol registry.** A primitive that needs state owns it. Nothing needs a
  globally unique offset, so nothing can exhaust a global table.
- **No `Wire`.** Passing an input *is* the binding. The clearest evidence of how
  much ceremony this removes is today's `equation.CausalBaseline`, which needs a
  four-line `Wire` block wrapped around `Identity` purely to say "the mean is
  the baseline."
- **No prefixes.** Two instances of `Window` are two objects, not two reserved
  regions of one address space.
- **No ceiling.** A primitive holding its own samples holds a real growable
  slice.

### 1.1 The boxing trap **[proto]**

The naive encoding of this interface is a performance disaster, and it must be
designed around from the start rather than discovered later:

```
BenchmarkClosedIface-16     206282416    5.623 ns/op    8 B/op    1 allocs/op
BenchmarkStructFunc-16      742031090    1.602 ns/op    0 B/op    0 allocs/op
```

Every `Step` that returns a `float64`-shaped value boxed into an interface
allocates. At the tick rates this engine runs, that is fatal. The contract is
right; the *representation* underneath it has to avoid boxing on the hot path.
See §3.

---

## 2. Atomicity: the Decay rule **[settled]**

Decay is the worked example that sets the standard for how small a primitive
should be. On its own, `Decay` does not decay — it zeroes:

```
Input -> [A slot] -> [B slot] -> Output      // A: Clock, B: Exponential
```

and in general, for every node in the package:

```
Input -> slots... -> Output
```

- Both slots empty → the value jumps straight to `0`.
- Clock filled → linear decay.
- Clock + exponential filled → exponential decay as normally understood.

Earlier drafting of this document said "Decay is not a primitive." That was
wrong, and the friction it caused is real information: `Decay` *is* a
meaningful, nameable thing — it is just not the same *kind* of thing as
`Product`. The vocabulary was missing a layer, not the design. See §2.5.

### 2.1 The zero-value rule **[settled]**

Prototyping this surfaced a real constraint that is easy to get backwards.

The obvious encoding has the clock slot supply *progress*, defaulting to `0`.
But then an empty clock gives `remaining = 1 - 0 = 1`, and the value passes
through **unchanged** — the opposite of the specified behaviour. (My first
prototype did exactly this and failed its own test: `empty slots must jump to
zero, got 100`.)

So the rule is:

> A slot's zero value must be chosen so that an empty slot yields the
> **degenerate** behaviour of the composition.

Which means slots compose the **retained fraction**, not the progress. Absent
clock = fully elapsed; absent shape = linear. Validated:

```go
Decay{}.Step(100)                      // 0                — degenerate
Decay{Clock: half}.Step(100)           // 50               — linear
Decay{Clock: half, Shape: exp}.Step(100) // 100*e^-0.5     — exponential
```

`BenchmarkDecayEmpty 1.684 ns/op · 0 allocs` /
`BenchmarkDecayComposed 4.354 ns/op · 0 allocs`.

This rule is a design obligation on every primitive, and belongs in the
contributor docs: *state what your zero value degenerates to, and prove it in a
test.*

### 2.2 Naming conventions **[settled — proto]**

Three conventions, applied without exception, are what make the package
*learnable*: a reader who knows the conventions can read a node they have never
seen before.

**C1 — values always fit slots `A`, `B`, `C`, …**

Position carries the meaning; each type documents what its positions are. There
are no bespoke field names per primitive.

```go
/*
A: Clock
B: Exponential
*/
nomagique.Number(
    &Decay{
        A: Const(0.5),
        B: Exponential{},
    },
)
```

This is why the universal shape is `Input -> slots... -> Output`. It also means
composition is structural: any slot takes any node, and nothing needs to know a
counterpart's vocabulary.

**C2 — `Type` always names an enumerated variant** of the thing it is on, never
anything else. `Store{Type: FIFO}`, and by extension `Store{Type: LIFO}`. A
reader seeing `Type:` knows immediately it is choosing among named variants of
that node, not configuring behaviour.

**C3 — one key names the branching condition**, everywhere (see §2.3).

**A consequence worth stating: the zero value is the operation's identity.**

C1 sharpens §2.1. Because every slot is positional and optional, "what does an
empty slot contribute" must be answered per type — and the general answer is
*the identity element of that node's operation*:

| Node | Empty slot contributes | Why |
| --- | --- | --- |
| `Sum` | `0` | additive identity |
| `Product` | `1` | multiplicative identity |
| `Decay` A (clock) | `1` (fully elapsed) | so an unclocked decay zeroes |
| `Decay` B (shape) | linear | the un-shaped default |

Verified: `Sum{A: Const(5)}` → `5` and `Product{A: Const(5)}` → `5`. Both
"ignore" the empty operand, but only because each uses its own identity. Getting
this backwards is exactly the trap §2.1 documents.

### 2.3 Branching: one node, one condition key **[proto]**

`Tee` and `Blend` are the same thing. This was not obvious until the slot
convention was applied to them, which is a good sign the convention is doing
real work.

Both take sub-graphs in `A`, `B`, …; both run them on the same input; both
combine the results. The *only* difference is how much of each branch flows —
which is exactly what a branching condition answers. So there is one node:

```go
type Split struct {
    Weight Weigher   // the one branching-condition key. Empty => all branches.
    A, B, C, D Node
}
```

The condition returns a **weight per branch**, and the result is the weighted
sum. Every branching form is then a setting of that one key:

| Weight | Behaviour | Classic name |
| --- | --- | --- |
| *empty* | every branch runs, weight 1 | **Tee** |
| `1` or `0` per branch | one branch passes, others shut | **Gate** |
| fractional, sums to 1 | branches mix continuously | **Blend** |
| `1` on the chosen index | n-way routing | **Mux** |

All four verified from the single `Split` type, 0 allocs
(`BenchmarkSplitTee 29.49 ns/op`):

```go
// Tee — empty condition, both branches run and are summed
Split{
    A: Sum{
        A: &Decay{A: &Clock{Interval: 1 * time.Millisecond}},
        B: &Decay{A: &Clock{Interval: 2 * time.Second}},
    },
    B: &Store{Type: FIFO},
}

// Mux — n-way, same node
Split{
    Weight: Pick{Index: bucket},
    A: Const(1), B: Const(2), C: Const(3), D: Const(4),
}
```

Note the empty-condition case obeys the zero-value rule (§2.1): absent condition
= no filtering = everything flows. The degenerate behaviour of a branch node is
"do not branch", which is precisely a tee.

**Open — arity.** Fixed `A, B, C, D` caps branches at four. Options: (a) accept
a fixed maximum, (b) a variadic `Slots []Node` escape for the rare n-way case,
(c) generate wider variants. (a) keeps literals clean and covers nearly every
real case; (b) reintroduces a second way to say the same thing, which the
conventions exist to prevent. **[open]**

**Open — the condition key's name.** `Weight` describes the mechanism honestly
(it returns weights) and generalises past boolean branching, but reads oddly for
the Mux case. Alternatives: `When`, `Select`, `Route`. Whatever it is, C3 says
it must be the *same key everywhere*. **[open]**

### 2.4 Slots solve multi-input **[proto]**

A single-value carrier appears to break for binary primitives (correlation needs
two series). It does not: the second operand is a **slot**, filled at
construction, exactly like Decay's clock. The primitive stays unary over the
carrier, so it still composes in one pipeline.

```go
type Corr struct { Peer Src; /* running stats */ }
func (c *Corr) Step(x float64) float64 {
    if c.Peer == nil { return 0 } // zero-value rule
    ...
}
```

`BenchmarkBinaryAsSlot 20.89 ns/op · 0 allocs`. Absent peer degenerates to `0`.

### 2.5 The missing layer: a taxonomy **[open — naming]**

The friction is real and it is not a maths gap. Calling `Decay` "not a
primitive" is unsatisfying because `Decay` *is* a real, nameable, reusable
thing. What is actually true is that today's single word `Primitive` is being
made to cover three genuinely different kinds of object.

The distinction is not invented — it already exists in the current package, it
just has no name. Sorting the existing inventory by *what a thing holds across
ticks* separates it cleanly:

| Kind          | Holds across ticks                        | Arity        | Examples from today's package                                                                             |
|---------------|-------------------------------------------|--------------|-----------------------------------------------------------------------------------------------------------|
| **Operation** | nothing                                   | fixed (1–2)  | `Product`, `Sum`, `Quotient`, `Difference`, `Negative`, `Absolute`, `Exp`, `Log`, `Minimum`, `Maximum`    |
| **Reduction** | nothing (folds a collection it is handed) | a collection | `Mean`, `Median`, `Deviation`, `Maturity`, `Kish`, `MedianAbsolute`                                       |
| **Primitive** | **its own state**                         | 1 + slots    | `Decay`, `Accumulate`, `Rate`, `Attack`, `Window`, `Baseline`, `Velocity`, `ZScore`, `Slope`, `Stability` |

Measured, not guessed. Counting cross-tick state references in the current
source: `mean` 0, `median` 0, `deviation` 0, `maturity` 0, `kish` 0 — versus
`stability` 4, `slope` 12, `zscore` 11, `velocity` 15, `baseline` 19. And in
`calculus`, `product`/`sum`/`quotient` are pure arity-2 functions, while
`decay` carries `Level`, `accumulate` carries `Total`, `rate` carries `Count`.

The line is sharp, and it is the *right* line, because it is exactly the line
that decides who owns memory in v2 (§3 rule 2).

**Proposed vocabulary:**

- **`Operation`** — a pure function on values. No identity, no memory, no
  slots. `Product`, `Exp`, `Negative`. This is the true atom: the thing that
  cannot be decomposed further because it *is* the arithmetic.

- **`Reduction`** — a pure fold from many values to one. Also stateless: it
  does not retain the collection, it is *handed* one. `Mean`, `Median`. Kept
  distinct from `Operation` because its arity is a collection rather than a
  fixed number of operands, which matters for how it composes.

- **`Primitive`** — a thing with identity: it owns state across ticks, and it
  has **slots** that shape its behaviour. `Decay` is a primitive. So is
  `Window`, `Baseline`, `Accumulate`. This is the layer directly beneath
  equations, and it is where the Decay rule (§2, §2.1) applies.

So the corrected statement is:

> `Decay` is a **Primitive**. It is composed *of* Operations (a `Product`, an
> `Exponential`) via its slots, and it owns a retained `Level`. What it is not
> is an **Operation** — and that is the distinction the old single word was
> hiding.

This resolves the friction cleanly: Decay behaves like a primitive to a caller
(nameable, reusable, meaningful), while still being *composed* internally
rather than implemented. Both intuitions were right about different layers.

**Consequences worth stating:**

1. **Only `Primitive` owns memory.** `Operation` and `Reduction` are pure, so
   they are trivially shareable, trivially concurrent-safe, and can be inlined
   or constant-folded at composition time. This is a large performance lever
   (§3) that the current design cannot pull, because `func(*Frame)` makes
   everything look stateful whether it is or not.
2. **Only `Primitive` needs the zero-value rule** (§2.1) — the rule is about
   slots, and only primitives have slots.
3. **Equations compose all three**; they still hold no implementation (§5).
4. The full stack becomes:
   `Operation` / `Reduction` → `Primitive` → `Equation` → `Algo`.

**Open:** the names. `Operation` vs `Atom` vs `Op`; `Reduction` vs `Fold` vs
`Aggregate`. `Operation`/`Reduction`/`Primitive` reads well and matches the
existing package split (`calculus` is almost exactly Operations, and the pure
half of `statistic` is almost exactly Reductions), but this is a naming call,
not a structural one — the three-way split is what matters.

**Also open:** whether `Reduction` earns its own kind or is just an `Operation`
with collection arity. Argument for keeping it separate: it is the natural
consumer of a `Window`'s contents, and that pairing (`Window` + `Mean`) is the
single most common shape in the whole package.

---

## 3. Performance **[proto]**

Requirement 4 is not a constraint to satisfy afterwards; it decides the
representation. Measurements from the prototype, against today's engine:

| Shape                                           | ns/op | allocs |
|-------------------------------------------------|-------|--------|
| today: `Number.Step` (97 KB Frame, 1 primitive) | ~9200 | 0      |
| closed iface, boxed                             | 5.6   | **1**  |
| unary func over `Number` (4-stage pipeline)     | 8.7   | 0      |
| stateful chain, 3 nodes incl. 64-sample mean    | 26.3  | 0      |
| same, monomorphised (no iface dispatch)         | 22.9  | 0      |

The headline: a stateful 3-node chain doing *more real work* than today's
single-primitive `Number.Step` runs ~350× faster, because there is no
fixed-width Frame to copy. Today ~77% of `Number.Step`'s profile is `memmove`
of the Frame; that entire cost is an artifact of the design, not of the maths.

Design rules that follow:

1. **Never box the carrier on the hot path.** Whatever satisfies the public
   closed interface, the inner loop must run over a concrete type. Options:
   generics with concrete instantiation, monomorphic composed structs, or a
   compile/"link" step that flattens a composition into a concrete chain.
   **[open]**
2. **State is owned, not shared.** Each stateful primitive holds its own slice.
   No global table, no fixed width, no `Reset` of a 97 KB array.
3. **Composition is built once, stepped many times.** All wiring, validation and
   slot resolution happens at construction; `Step` is arithmetic only.
4. **Measure allocations in CI.** The current package has `AllocsPerRun(...) == 0`
   assertions and they were load-bearing — they caught the boxing regression
   immediately. Keep that discipline, but assert on the *engine*, never on a
   caller-side copy (see §7).

---

## 4. Ergonomics: `Number` as a number **[open, strongly desired]**

The nice-to-have that makes the whole package feel native:

```go
nomagique.Number(...) * 0.25
```

If the carrier is `type Number float64`, this works with no wrapper, no
`.Value()`, no unboxing — and ordinary arithmetic composes with pipeline
results. **[proto]** confirms it costs nothing:

```
BenchmarkNumberSeq4-16     130885780    8.672 ns/op    0 B/op    0 allocs/op
BenchmarkNumberArith-16    277374236    4.311 ns/op    0 B/op    0 allocs/op
```

The unresolved tension: a bare `float64` cannot carry a window's samples, an
event clock, or a covariance accumulator.

**Candidate resolution — separate the carrier from the node.** The value
flowing between primitives is `Number` (a `float64`). *State lives in the
primitive instance*, not in the value. This is what makes both requirements
satisfiable at once, and it is what the prototype in §3 row 4 actually does.

### 4.1 Can generics remove `Step` entirely? **[proto — answered: partly]**

The question: if `Primitive` were generic — `type Decay[T comparable] struct{}`
— could we drop `Step` and just use ordinary math operators?

**`comparable` is the wrong constraint.** It admits only `==` and `!=`. Verified
against the compiler:

```
invalid operation: operator * not defined on v
    (variable of type T constrained by comparable)
```

The constraint that admits arithmetic is a type-set union, `~float64`:

```go
type Real interface{ ~float64 }
func Scaled[T Real](v, k T) T { return v * k }   // compiles
```

**What genuinely works with no `Step` and no interface** — all validated at
0 allocs:

```go
type Number float64

n := pipeline.Apply(x)      // returns Number
n * 0.25                    // ordinary arithmetic, no wrapper
math.Sqrt(float64(n))       // stdlib interop
Scaled(n, 2)                // generic ops over ~float64
```

`BenchmarkPureValue 2.733 ns/op · 0 allocs`.

**Where it stops.** A `float64`-shaped type is 8 bytes and has nowhere to put a
growable window, an event clock, or a covariance accumulator. Adding them makes
it a struct (measured: 8 → 32 bytes with one slice), and the moment it is a
struct, `x * 0.25` stops compiling and stdlib math needs unwrapping. You cannot
have unbounded state *and* float64 value semantics in the same type. This is a
hard constraint, not an implementation gap.

**Resolution — the split is the answer to both questions.**

> The **value** that flows is `Number` (float64-shaped, arithmetic-native).
> The **state** is owned by the node, which is a separate object.

So `Step`/`Apply` does not disappear — but it stops being a protocol wrapper and
becomes just "the node's transfer function." Stateless Operations and Reductions
(§2.5) genuinely need no method at all: they are plain generic funcs over
`~float64`. Only stateful Primitives need one, and only because they must be
addressed to mutate.

Validated together in one test — result is float64-shaped, stdlib math works,
generic ops work, state is unbounded, 0 allocs
(`BenchmarkSplitValueState 10.71 ns/op`).

This also sharpens §2.5: **the taxonomy predicts which layer needs a method.**
Operations and Reductions are pure functions; Primitives are objects. That is
the same line, arrived at independently.

Open questions:
- How does a composition expose "the current value" for arithmetic while still
  being steppable? Likely: the composition is a node you keep, and `Apply`
  returns `Number`. **[open]**
- Multi-output primitives (a z-score that also wants to publish dispersion and
  readiness). Slots for *outputs* too? Or a composition returns `Number` and
  auxiliary readings are pulled from the instance on demand? **[open]** — this
  is the biggest unresolved question in the design, and note it is *not* solved
  by the value/state split: the split says auxiliary readings live on the node,
  but not how a pipeline exposes them without becoming a bag of slots again.

---

### 4.2 What it actually looks like **[proto — every example compiles and passes]**

The requirement is that anything the primitives can express is reachable as a
**single `nomagique.Number(...)` expression** — one nested literal, not a
procedure that builds a graph out of intermediate variables.

That is achievable, and it turns on two constructs.

#### The two constructs that make it work

**`Split`** — the one branching node (§2.3). With an empty condition every
branch runs, so a side-effecting path (capture, store, publish) needs no named
variable:

```go
type Split struct {
    Weight Weigher   // empty => all branches run
    A, B, C, D Node
}
```

**Owned sub-state** — a Reduction that *contains* its store rather than
pointing at one. This is what removes the last intermediate variable: without
it, `Mean{Of: w}` forces `w` to be declared separately.

```go
type Over struct {
    Store  Store                  // owned by value, declared inline
    Reduce func([]Number) Number
}
```

#### Scenario A — the sketch, working

This is the shape from the brief, and it runs as written:

```go
nomagique.Number(
    Split{
        A: Sum{
            A: &Decay{A: &Clock{Interval: 1 * time.Millisecond}},
            B: &Decay{A: &Clock{Interval: 2 * time.Second}},
        },
        B: &Store{Type: FIFO},
    },
)
```

Two decays on different clocks, summed, with every input captured to a FIFO on
the side. First tick `200` (both levels fresh); after 2 ms the fast decay has
collapsed and the slow one has barely moved, giving `99.88`. The store holds
both inputs.

#### Scenario B — a smoothed price, one literal, no variables

```go
n := nomagique.Number(
    &Over{
        Store:  Store{Type: FIFO, Span: 4},
        Reduce: Average,
    },
)

// feed 10,12,14,16
out := n.Apply(16)      // 13
quarter := out * 0.25   // 3.25 — still a float64
```

#### Scenario C — Decay, with a nested shape

Slots take sub-graphs, so shaping is composition rather than configuration:

```go
/*
   A: Clock
   B: Shape
*/
nomagique.Number(
    &Decay{
        A: Const(0.5),
        B: Exponential{},
    },
)
// 100 -> 60.653  (= 100·e^-0.5)
```

Drop `Shape` and it is linear; drop both and it zeroes (§2.1).

#### Scenario D — branching, still one expression

No `if`, no separate wiring step. The condition is a node like any other:

```go
nomagique.Number(
    Split{
        Weight: Above{Threshold: 50},
        A: &Over{Store: Store{Type: FIFO, Span: 2}, Reduce: Average},  // fast
        B: &Over{Store: Store{Type: FIFO, Span: 8}, Reduce: Average},  // slow
    },
)
```

Feeding `10,20,30,100`: the last input exceeds the threshold, so weight is 1 and
the fast regime wins — mean of `{30,100}` = `65`. With a continuous `Weight`
the two regimes mix instead of switching, and a hard branch is just the
degenerate case where the weight is pinned to 0 or 1 — which is why `Split`
subsumes tee, gate, blend and mux alike (§2.3).

`BenchmarkDeclarative 24.76 ns/op · 0 allocs` for Scenario A's graph.

#### What this costs

Two honest caveats:

1. **Pointers on stateful nodes.** `&Decay{...}` and `&Over{...}` need the
   ampersand because they mutate. Stateless nodes (`Sum`, `Blend`, `Const`,
   `Exponential`, `Above`) nest as plain values. The split is at least
   *meaningful*: the `&` marks exactly the things that carry state, which is the
   §2.5 line showing through in the syntax. But the cost is real and measured —
   see §4.3.
2. **Reading auxiliary results still needs a handle.** Scenario A keeps `store`
   in a variable only to inspect what it captured afterwards. That is the
   multi-output problem (§4) surfacing again in the declarative form, and it is
   still the biggest open question.

#### A warning: the escape hatch **[open]**

Writing a `ZScore` equation, the version that works wraps raw arithmetic and a
hand-rolled `stddev` in a `Func(...)` adapter. **That violates §5** — it is an
equation carrying its own implementation, the same sin as today's
`equation.CausalBaseline` smuggling logic through a `Wire` block.

Given `Center` and `Dispersion` primitives it should instead read as pure
composition:

```go
nomagique.Number(
    &Standardize{
        Center: &Over{Store: Store{Span: 32}, Reduce: Average},
        Scale:  &Dispersion{Store: Store{Span: 32}},
    },
)
```

Conclusions: `Func` should probably not be public, or should be confined to
Operations; and §5's missing-primitive test is load-bearing — *if an equation
needs `Func`, a primitive is missing*. Whether that can be enforced rather than
merely stated is **[open]**.

---

### 4.3 Pointers: measured, not assumed **[proto]**

The declarative form above uses `&` on every stateful node, which quietly
creates one heap object per node per stream. At symm's scale (~2200 keyed
streams) that is worth measuring rather than assuming, because unexamined
per-node overhead is exactly how the current engine ended up where it is.

Three encodings of the *same* graph, all producing identical results
(verified by an equivalence test):

| Encoding | step (1 stream) | step (2200 streams) | heap objects | GC pause | build (2200) |
| --- | --- | --- | --- | --- | --- |
| **A. pointer nodes + interface** | 23.7 ns | 27.7 ns | 13,197 | 248 µs | 493 KB / 13,200 allocs |
| **B. shared spec + state arena** | 25.7 ns | 29.0 ns | 6,596 | 181 µs | 246 KB / 8,800 allocs |
| **C. equation embedded by value** | **19.2 ns** | **20.3 ns** | **2,191** | 283 µs | 268 KB / 2,200 allocs |

All three are **0 allocs/op in steady state** — the hot path is clean in every
case. The differences are in *construction* and in what the GC has to trace.

**Findings:**

1. **Steady-state step cost barely moves** (19–29 ns), and cache locality across
   2200 streams is not the problem it might have been: A and B differ by ~1 ns
   at scale. Pointer-chasing is not the enemy here.
2. **Option B is not worth it.** Sharing one immutable spec across all keys and
   indexing into a per-key state arena halves the objects, but costs a level of
   indirection, a two-phase `Alloc`/`Eval` protocol, and slot-index bookkeeping
   — for *no* steady-state gain and a slower step than C. This is the kind of
   machinery that looks clever and buys nothing; it is recorded here mainly so
   it is not re-invented.
3. **Option C wins on the metric that actually scales.** Embedding a
   composition's children *by value* in a concrete struct gives 6× fewer heap
   objects than A and the fastest step, because the compiler can inline straight
   through — no interface dispatch, one allocation for the whole equation.

**The rule this suggests:**

> Nested literals with `&` are the *authoring* form. A published **Equation**
> should be a concrete struct with its children embedded by value.

That is a pleasing result, because it is the same layer boundary as §2.5 and §5:
an Equation is a preset composition, and "preset" is precisely what licenses the
compiler to flatten it. Ad-hoc graphs stay flexible and pay a little; the
common shapes get named, embedded, and become fast. It also gives §3's "flatten
a composition into a concrete chain" a concrete meaning.

**Still open:** whether that flattening can be mechanical (code generation from
a declarative spec, so authors write option A and ship option C) or whether
equations are simply hand-written structs. **[open]**

Note the GC-pause column is the one place C does not win outright (283 µs vs
B's 181 µs) — fewer, larger objects rather than more, smaller ones. At these
magnitudes it is noise, but it is the number to watch if graphs get much bigger.

## 5. The full stack: equations and algos **[settled]**

Layered, from the taxonomy in §2.5:

```
Operation / Reduction  →  Primitive  →  Equation  →  Algo
   (pure, no memory)      (owns state,   (preset      (preset
                           has slots)   composition) composition,
                                                      named + cited)
```

- **Operations / Reductions** — pure. No memory, no slots. The true atoms.
- **Primitives** — own state, expose slots. `Decay`, `Window`, `Baseline`.
  Composed *of* Operations via their slots, never implementing arithmetic
  themselves.
- **Equations** — *preset compositions only*. No implementation of their own,
  not one line of arithmetic that is not itself an Operation or Primitive. If an
  equation needs to compute something no lower layer provides, that is a missing
  Operation or Primitive.
- **Algos** — same rule, but reserved for *well-known named algorithms*
  (Hawkes, Pearl, Welford, Hayashi-Yoshida). An algo is a citation, not an
  invention.

This is already the stated intent; today's `equation.CausalBaseline` violates it
by carrying a `Wire` block, and that violation disappears with the closed
contract.

---

## 6. Logic and gating **[open]**

`If(predicate, whenTrue, whenFalse)` is imperative control flow smuggled into a
dataflow package, and it forces a Frame copy per branch to stay transactional.

The functional form already exists in the package and is the better model —
`logic.Gate` is `value × condition`, no branch:

```go
result = value * nonZero(condition)
```

Direction: gating and routing should be **arithmetic on the carrier**, so a
"branch" is a blend/mask rather than a jump. `Mux` generalises this to n-way
selection. Everything stays differentiable-ish, allocation-free, and composable.

Scenario D in §4.2 makes this concrete and suggests an ordering: **`Blend` is
the general primitive and `Gate` is its special case** (weight pinned to 0/1),
not the other way around. A hard `If` is then simply the most degenerate point
of a continuum the package can express natively.

Open: whether any genuinely short-circuiting construct is still required (for
cost, not semantics — skipping an expensive subtree when a mask is zero), and
if so how to express it without reintroducing `If`. **[open]**

---

## 7. Lessons to carry over

Hard-won from the current implementation; re-learning these would be expensive.

- **`MaxSlots` *is* the Frame width.** In the current design the registry
  capacity and the struct width are the same number and cannot be tuned apart.
  Raising the ceiling to stop the panic (128→192 mask words) works, but it
  trades slots against `memmove` on the hot path and entrenches the magic
  number. It is scaffolding, not a fix. See memory `symbol-registry-capacity`.
- **Alloc assertions must target the engine.** Three
  `AllocsPerRun(...) == 0` tests broke when `Frame` got wider. They were *test*
  artifacts: binding a large return value to a local inside the measured closure
  makes the compiler heap-allocate the caller's copy. `Wire`, `Number.Step` and
  `NewSingle` still allocated zero. Fix such a test by not binding the return —
  never by relaxing the assertion.
- **Value receivers on a large struct are a silent tax.** `MustGet`, `Has`,
  `Count`, `All`, `Finite`, `Equal` had value receivers and `Merge`/
  `MergeChanged`/`Equal` took value parameters — each copying the whole backing
  array per call, on hot paths. In v2 the carrier is small enough that this
  stops being a hazard, which is itself an argument for the design.
- **The adaptive control loop itself is sound.** Grow on a stability dip, shrink
  at perfect stability, slide otherwise (`temporal/governor.go`). Only the
  clamps are wrong. Port the loop, drop the ceiling.

---

## 8. Open questions, consolidated

0. **The taxonomy names** — `Operation`/`Reduction`/`Primitive`, and whether
   `Reduction` is its own kind or an `Operation` with collection arity. The
   three-way split is settled; only the naming is open. (§2.5)
1. **Representation under the closed interface** that avoids boxing on the hot
   path — generics, monomorphised composites, or a flatten/link step. (§3.1)
   Note this now interacts with §2.5: `Operation` and `Reduction` are pure, so
   they can be inlined or constant-folded at composition time in a way
   `Primitive` cannot.
2. **Multi-output primitives** — the largest unresolved question. (§4)
3. **Composition-as-value vs composition-as-function** — how `Number(...)` both
   yields a float and remains steppable. (§4)
4. **Short-circuiting** — needed for cost? expressible without `If`? (§6)
5. **Keying** — today `Number[Key]` isolates per-symbol state. Under owned
   state, is a keyed composition just a map of instances, and who owns
   lifecycle/eviction?
6. **Concurrency** — the current engine serialises per key with a mutex and a
   scratch frame. With owned state, what is the threading contract for a single
   composition instance?
7. **Migration** — ~31k prod LOC in `nomagique` (+12k tests), 36 dependent
   packages, 795 `MustIntern` sites, 282 `Wire` sites, 480 `Frame` refs. Big
   bang, or a v2 package that signals migrate to one at a time?

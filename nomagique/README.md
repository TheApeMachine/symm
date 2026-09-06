# Primitive subset: implementation notes

This directory is a **partial conversion** to the Primitive protocol. The current constructor compositions and their tests define the migrated paths. The remaining Step-based consumers are not yet compatible with them.

```go
type Primitive interface {
    Next(Primitive) Primitive
    Read() any
    Error(...error) error
}
```

A caller consumes a delivery run until nil. Nil closes that run, not necessarily all future runs. Nil input offers no upstream data; the callee determines its response.

`core.From` creates an inert carrier. `Proto.Next` returns nil. `transport.IO` explicitly presents configured Primitives into a finite run without inspecting them.

```go
sum := arithmetic.NewAdd[float64](
    transport.NewIO(core.From(0.0)),
)
input := transport.NewIO(core.From(2.0), core.From(3.0))
```

`sum.Next(input)` yields the sum; its next call closes that delivery run. A new run uses the same configured seed source. The last result remains available through `Read`.

For stateful accumulation, the same Add is wired through retention:

```go
memory := store.NewRetained(core.From(0.0))
running := transport.NewMap(
    transport.NewPipe(
        arithmetic.NewAdd[float64](memory),
        memory,
    ),
)
```

No Add-specific accumulation mode exists. The configured left connection is never replaced by an answer.

`core.Yield[A,B]` is the typed fold boundary. It requests a seed from its left source, drains its right source, and carries errors. The source determines when a run ends. Primitive-valued folds transport objects without reading their payloads.

A fixed-seed KV creates a fresh record per run; a KV wired through Retained maintains keyed state. Maps contain Primitive values, not a second metadata object model. `Read`/`To` are Go boundaries; callers must respect ownership of values they read.

`nomagique.Number`, equations and named algorithms construct Primitive graphs. They do not execute the graph during construction. Map owns per-value application; a reducer consumes a whole run. Fan shares one captured incoming run through separate cursors. Batch frames a configured source without discarding the next value.

The implementation uses finite-run buffering and serialized graph ownership. It makes no zero-allocation, concurrency or live-market throughput guarantee.


## Continuation, pass 2

The graph now includes adaptive causal baselines, weighted priors, reward
measurement, square-root RLS, linear/causal fits, joint moments, Hawkes
likelihood/gradients, and calibration/quality compositions. Model/Grid/runtime,
legacy measurements, correlation consumers and full Hawkes fitting are still
not migrated.

For a separately updated retained source used in an input-evaluated expression,
bind its nil-input query explicitly:

```go
band := store.NewRetained(core.From(0.5))
target := learning.NewDirectionalTarget(transport.NewApply(band, nil))
```

Passing `band` directly instead would ask it to retain the expression's input.
No type-specific interpretation of Retained is added to a caller.

`go run ./examples/learning` from the module root shows the causal baseline and
an RLS stream whose last row is prediction-only. Its coefficient state does not
change merely because a forecast was requested.


## Dependency direction during migration

Equations compose arithmetic, calculus, logic, collections and transport.
Algorithms compose equations. Adaptive policies configure algorithms and
estimators. Lower layers must not import adaptive policies to choose defaults.

The old equation-level baseline and joint implementations have been replaced by
`adaptive.NewBaseline(window)`, `equation.NewCausalResidual(moments)` and
`joint.NewEstimator(estimators)`. Their state and output records are exercised
by the existing causal-residual and joint reference tests. Application callers
still using the removed Step APIs require migration to these records.

`joint.NewDivergence(estimators, regressions)` combines joint residuals and
per-coordinate regressions. Both endpoint collections are supplied by the
caller; there is no fixed channel count or implicit keyed state. Each input
contains `values` in log space and an exact nanosecond `at`. The output contains
`channels` and `velocities` in matching order. Regressing time is rejected before
any estimator advances.

`equation.NewRenewalRate(target)` receives records containing `increment`,
`sample` and nanosecond `at`. Its target is a supplied quantity estimator, so it
does not import an adaptive policy. Every observation yields a record with
`closed`, `rate`, `change` and `maturity`. Missing fields and invalid domains
are errors. A retained rate on an unfinished span is distinguished by `closed`.

The covariance normalization and configured lag scan live in
`equation.NewCorrelation` and `equation.NewLagProfile`. The named asynchronous
estimator remains `algo.NewHayashiYoshida`. Formula/algorithm integration tests
use external test packages, so they do not create reverse package dependencies.

The unused Step-based vector classifier, volatility router and stability
controller have been removed. The Hawkes forwarding type was also removed;
its signal caller now uses the existing `statistic/hawkes.Bivariate` owner
directly. This does not claim that Hawkes fitting or its measurement boundary
has been migrated.

An acyclic import graph does not establish that the entire application builds.
Use `go list -test ./nomagique/...` to inspect dependencies, and run the package
tests to expose the remaining API migration work. The new graph implementations
still buffer delivery runs and allocate; they have no live Level3 throughput
certification.

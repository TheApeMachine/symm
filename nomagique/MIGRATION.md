# Migration to explicit wiring

The outer `Frame` is now the namespace for facts and persistent state. An atom
must not inspect that namespace directly. Wrap atoms with `Wire` and list every
input, output, and state binding:

```go
change := nomagique.Wire(
    calculus.Difference,
    nomagique.In(CurrentPrice, calculus.PortA),
    nomagique.In(PreviousPrice, calculus.PortB),
    nomagique.Out(calculus.PortResult, PriceChange),
)
```

There is no fallback slot resolution. A missing bound fact rejects the
transition. Local state is isolated in the same way:

```go
counter := nomagique.Wire(
    localAccumulator,
    nomagique.In(Increment, LocalDelta),
    nomagique.State(CommittedCount, LocalTotal),
    nomagique.Out(LocalResult, Count),
)
```

Use `ForkStrict` for new fan-out equations. All branches observe the same base
snapshot, and collisions are errors. Use `Number[K]` as the keyed state owner;
`KeyedStreams` remains only as a compatibility facade.

Compatibility surfaces retained for staged migration are `Relay`,
`KeyedStreams`, readiness-gated `calculus.Ratio`, and the legacy reciprocal
`calculus.Inverse`. New equations in this tree no longer depend on them.

/*
Package nomagique is a small algebra for stateful numeric computation.

The name is the rule it exists to enforce: no magic numbers. No primitive
accepts a hardcoded operational constant. Every parameter that shapes, bounds,
filters, or attenuates a stream is an adaptive dynamic derived causally from
the data itself — windows that expand while a stream is stationary and
contract when drift is detected, thresholds that calibrate to running
dispersion, clocks that decay in information time rather than calendar
milliseconds, envelopes that establish frontiers from distribution tails.

Its boundaries are deliberately strict:

  - Scalar is the carrier: a defined float64, unboxed, participating natively
    in Go arithmetic without unwrapping.
  - Node is the closed contract every transformation satisfies:
    Step(Scalar) Scalar. Because Step consumes and returns the carrier, any
    node — or any composition of nodes — can feed any other.
  - Chain composes in series, Split in parallel. Split's Route key subsumes
    gating, switching, and blending, so no separate Tee, Gate, Mux, or Blend
    primitive exists.
  - Nodes own their state privately, as ordinary Go struct fields and growable
    slices. There is no global registry, no interned symbol table, and no
    shared address space between instances.
  - Number composes a graph into a runnable Pipeline.

Logic is stratified by how much memory it retains across ticks: pure
Operations and Reductions hold none, Primitives own state, Equations compose
lower tiers and contain no arithmetic of their own, and Algos are reserved for
established published algorithms.

A node knows structural roles — first operand, second operand, rate, shape,
center, scale — never domain meaning. Domain meaning belongs at the boundary
that composes those roles. This keeps the arithmetic reusable for prices,
durations, quantities, sensor readings, or deliberate nonsense without
semantic coercion.
*/
package nomagique

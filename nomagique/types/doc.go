/*
Package types defines the carrier and the topological algebra every other
nomagique package composes against.

  - Scalar is the carrier: a defined float64, so it is 8 bytes, unboxed, and
    usable with +, -, *, / without unwrapping.
  - Node is the closed contract: Step(Scalar) Scalar. Structural closure over
    the carrier is what makes any node substitutable for any other.
  - Chain evaluates its slots in series, Split in parallel under a Router.
    Sum and Product fold their slots. Identity passes the carrier through.
  - Reduction is a pure fold over a slice of carriers, retaining nothing.
  - Tap adapts a multi-output node's auxiliary reading back into a Node, and
    Probe captures a value inline as it flows.

Every node declares what an omitted slot degenerates to, and that behavior is
the algebraic identity of its operation: a missing Chain slot passes the
carrier through, a missing Split or Sum slot contributes 0, a missing Product
slot contributes 1. A composition is therefore always well defined, however
sparsely it is configured.

Nodes own their state privately as ordinary struct fields. No global registry,
interned symbol table, or shared address space exists between instances, so
two nodes of the same type never interfere.
*/
package types

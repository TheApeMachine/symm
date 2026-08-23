/*
Package nomagique is a small algebra for stateful numeric computation.

Its boundaries are deliberately strict:

  - Frame stores named facts and committed state.
  - Primitive describes one relation over a local coordinate system.
  - Wire explicitly binds outer facts to local ports and local ports back to
    facts. Primitives never guess that one fact is “close enough” to another.
  - Pipe, Fork, Configure, and the logic package compose relations.
  - Number owns one transactional composed state per key.

A primitive may know structural roles such as first operand, second operand,
value, result, or condition. Domain meaning belongs at the boundary that wires
facts into those roles. This keeps arithmetic reusable for prices, durations,
quantities, sensor readings, or deliberate nonsense without semantic coercion.
*/
package nomagique

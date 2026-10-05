/*
Package geometry provides phase-fingerprint primitives: a high-dimensional
complex PhaseDial, an evenly spaced angular PhasePath, Hermitian Overlap, and a
bounded, outcome-tagged Corpus of retained dials.

Everything is a streaming Primitive over an unsafe.Pointer wire. Payloads are
plain data types: PhaseDial is a []complex128 of rotational phase gradients,
and every command, query, and reading is a struct with exported fields only.

The math is pure (dial overlap/copy/normalize/rank); only the corpus's retained
entries need mutual exclusion for concurrent reads and writes, and the Corpus
primitive owns that mutex itself.
*/
package geometry

/*
PhaseDial is a high-dimensional complex vector of rotational phase gradients.
Each component is a complex amplitude; its magnitude is scale and its argument
is phase. It carries no encoding policy — callers project their source (e.g. an
oscillator lattice) into it directly. It is pure wire payload: no methods.
*/
type PhaseDial []complex128

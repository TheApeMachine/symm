/*
Package geometry provides phase and coordinate primitives: an evenly spaced
angular PhasePath, normalized Overlap, Normalize, Relaxation, Watershed, and a
bounded, outcome-tagged Corpus.

Everything is a streaming core.Primitive over an unsafe.Pointer wire. Primitives
receive coordinate values one by one and yield outputs one by one.
*/
package geometry

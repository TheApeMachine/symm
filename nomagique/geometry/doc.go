/*
Package geometry provides phase and coordinate primitives: an evenly spaced
angular PhasePath, normalized Overlap, Normalize, Relaxation, Watershed, and a
bounded, outcome-tagged Corpus.

Everything is a streaming core.Primitive over an unsafe.Pointer wire. Each
Next receives the data.Adapter and reads and publishes its native names
through it; there are no payload structs.
*/
package geometry

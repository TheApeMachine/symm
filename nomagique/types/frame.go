package types

import "github.com/theapemachine/symm/nomagique"

// Frame is the universal reducer state and payload.
type Frame = nomagique.Frame

// Symbol is an interned Frame offset.
type Symbol = nomagique.Symbol

// Primitive is the pure reducer contract.
type Primitive = nomagique.Primitive

// Stream owns one ordered single-writer reducer state.
type Stream = nomagique.Stream

// AtomicStream publishes reducer snapshots with compare-and-swap commits.
type AtomicStream = nomagique.AtomicStream

const (
	MaxSlots   = nomagique.MaxSlots
	MaxSamples = nomagique.MaxSamples
)

func Intern(name string) (Symbol, error) {
	return nomagique.Intern(name)
}

func MustIntern(name string) Symbol {
	return nomagique.MustIntern(name)
}

func SymbolName(symbol Symbol) (string, bool) {
	return nomagique.SymbolName(symbol)
}

func FrameFromNamed(values map[string]float64) (Frame, error) {
	return nomagique.FrameFromNamed(values)
}

func NewStream(primitive Primitive, initial Frame) *Stream {
	return nomagique.NewStream(primitive, initial)
}

func NewAtomicStream(primitive Primitive, initial Frame) *AtomicStream {
	return nomagique.NewAtomicStream(primitive, initial)
}

func NewKeyedStreams[Key comparable](
	primitive Primitive,
	initial func(Key) Frame,
) *nomagique.KeyedStreams[Key] {
	return nomagique.NewKeyedStreams(primitive, initial)
}

package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type MultiplexType uint8

const (
	AddressableMultiplex MultiplexType = iota
)

/*
Multiplex ...
*/
type Multiplex[T any] struct {
	*core.PrimitiveError
	mt       MultiplexType
	channels []core.Primitive
}

/*
NewMultiPlex ...
*/
func NewMultiplex[T any](
	multiplexType MultiplexType, channels ...core.Primitive,
) *Multiplex[T] {
	return &Multiplex[T]{
		PrimitiveError: core.NewPrimitiveError(),
		channels:       channels,
	}
}

/*
Multiplex ...
*/
func (mp *Multiplex[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, channel := range mp.channels {
			for out := range channel.Next(in) {
				if !yield(out) {
					return
				}
			}
		}
	}
}

package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Conn connects an addressable member to a distributed store grid.
It completes registration once before the first arrival, yielding
the established connection downstream.
*/
type Conn[T interface {
	core.Ordered[T]
	comparable
}, U any] struct {
	*core.PrimitiveError
	once    *Once
	address *Address[T]
}

func NewConn[T interface {
	core.Ordered[T]
	comparable
}, U any](
	host core.Primitive,
	interests ...U,
) *Conn[T, U] {
	stages := make([]core.Primitive, 0, 3)

	if len(interests) > 0 {
		stages = append(stages, sequence.NewValues(interests...))
	}

	address := NewAddress[T]()

	stages = append(
		stages,
		core.NewQuery[T, U](
			address,
			core.Identify,
		),
		host,
	)

	return &Conn[T, U]{
		PrimitiveError: core.NewPrimitiveError(),
		once:           NewOnce(nomagique.NewNumber(stages...)),
		address:        address,
	}
}

func (conn *Conn[T, U]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return conn.once.Next(in)
}

func (conn *Conn[T, U]) Identity() T {
	return conn.address.Identity()
}

func (conn *Conn[T, U]) Identify(identity T) core.Identifiable[T] {
	return conn.address.Identify(identity)
}

func (conn *Conn[T, U]) Connect(primitive core.Primitive) {
	conn.address.Connect(primitive)
}

package transport

import (
	"iter"
	"reflect"
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
	once *Once
}

func isNil(primitive core.Primitive) bool {
	if primitive == nil {
		return true
	}

	val := reflect.ValueOf(primitive)

	return val.Kind() == reflect.Pointer && val.IsNil()
}

func NewConn[T interface {
	core.Ordered[T]
	comparable
}, U any](
	host core.Primitive,
	interests ...U,
) *Conn[T, U] {
	if isNil(host) {
		return &Conn[T, U]{
			PrimitiveError: core.NewPrimitiveError(),
			once:           NewOnce(sequence.NewValues[U]()),
		}
	}

	stages := make([]core.Primitive, 0, 3)

	if len(interests) > 0 {
		stages = append(stages, sequence.NewValues(interests...))
	}

	stages = append(
		stages,
		core.NewQuery[T, U](
			NewAddress[T](),
			core.Identify,
		),
		host,
	)

	return &Conn[T, U]{
		PrimitiveError: core.NewPrimitiveError(),
		once:           NewOnce(nomagique.NewNumber(stages...)),
	}
}

func (conn *Conn[T, U]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return conn.once.Next(in)
}

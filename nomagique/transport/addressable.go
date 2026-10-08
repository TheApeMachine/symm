package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type Addressable struct {
	*core.PrimitiveError
	id    string
	conn  core.Primitive
	space []core.Primitive
}

func NewAddressable(
	id string, conn core.Primitive, space ...core.Primitive,
) *Addressable {
	return &Addressable{
		PrimitiveError: core.NewPrimitiveError(),
		id:             id,
		conn:           conn,
		space:          space,
	}
}

func (addr *Addressable) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if addr.Error() != nil {
			return
		}
		flow := in
		if addr.conn != nil {
			flow = addr.conn.Next(in)
		}
		if len(addr.space) == 0 {
			for ptr := range flow {
				if !yield(ptr) {
					return
				}
			}
			if addr.conn != nil {
				addr.Error(addr.conn.Error())
			}
			return
		}
		for _, primitive := range addr.space {
			if primitive == nil {
				continue
			}
			if msg, ok := primitive.(*data.Message); ok {
				if addr.conn != nil {
					flow = addr.conn.Next(msg.Next(nil))
				}
				continue
			}
			for ptr := range primitive.Next(flow) {
				if !yield(ptr) {
					return
				}
			}
			if addr.conn != nil {
				addr.Error(addr.conn.Error())
			}

			if err := addr.Error(primitive.Error()); err != nil {
				return
			}
		}
	}
}

package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// Map applies one scalar-stream primitive to fixed-arity records. An optional
// prefix is copied before every record (for shared coordinate parameters).
// No tuple pointers cross the wire. Malformed input or a child error produces
// no partial result from the run.
type Map struct {
	*core.PrimitiveError
	width  int
	prefix int
	target core.Primitive
}

func NewMap(width int, target core.Primitive, prefix ...int) core.Primitive {
	op := &Map{PrimitiveError: core.NewPrimitiveError(), width: width, target: target}

	if width <= 0 || target == nil || len(prefix) > 1 {
		op.Error(core.ErrShape)
		return op
	}

	if len(prefix) == 1 {
		op.prefix = prefix[0]
	}

	if op.prefix < 0 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Map) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		var values []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			values = append(values, *(*float64)(arriving))
		}

		if len(values) < op.prefix || (len(values)-op.prefix)%op.width != 0 {
			op.Error(core.ErrShape)
			return
		}

		var output []float64
		operands := make([]float64, op.prefix+op.width)
		copy(operands, values[:op.prefix])

		for offset := op.prefix; offset < len(values); offset += op.width {
			copy(operands[op.prefix:], values[offset:offset+op.width])

			for pointer := range op.target.Next(data.NewValue(operands...).Next(nil)) {
				if pointer == nil {
					op.Error(core.ErrShape)
					return
				}

				output = append(output, *(*float64)(pointer))
			}

			if err := op.Error(op.target.Error()); err != nil {
				return
			}
		}

		for pointer := range data.NewValue(output...).Next(nil) {
			if !yield(pointer) {
				return
			}
		}
	}
}

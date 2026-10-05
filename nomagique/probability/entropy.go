package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Entropy owns -sum(p log p). Zero mass contributes zero;
negative inputs produce a domain error.
*/
type Entropy struct {
	*core.PrimitiveError
	acc    float64
	input  data.Map[string]
	output data.Map[float64]
}

func NewEntropy() *Entropy {
	output := data.NewOutputMap()
	output.Values["entropy"] = 0

	return &Entropy{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("mass", "mass"),
		output:         output,
	}
}

func (op *Entropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			mass, ok := values.Values["mass"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			if mass < 0 {
				op.Error(core.ErrDomain)
				return
			}

			if mass > 0 {
				op.acc -= mass * math.Log(mass)
			}

			op.output.Values["entropy"] = op.acc

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

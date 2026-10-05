package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Ambiguity divides entropy by the entropy of an equal-mass distribution.
A one-member distribution has zero ambiguity by definition.
*/
type Ambiguity struct {
	*core.PrimitiveError
	values []float64
	total  float64
	input  data.Map[string]
	output data.Map[float64]
}

func NewAmbiguity() *Ambiguity {
	output := data.NewOutputMap()
	output.Values["ambiguity"] = 0

	return &Ambiguity{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Ambiguity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			val, ok := values.Values["value"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			op.values = append(op.values, val)
			op.total += val

			ambiguityVal := 0.0

			if len(op.values) > 1 && op.total > 0 {
				entropy := 0.0

				for _, elem := range op.values {
					probabilityVal := elem / op.total

					if probabilityVal > 0 {
						entropy -= probabilityVal * math.Log(probabilityVal)
					}
				}

				ambiguityVal = entropy / math.Log(float64(len(op.values)))
			}

			op.output.Values["ambiguity"] = ambiguityVal

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

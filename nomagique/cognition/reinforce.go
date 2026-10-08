package cognition

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Reinforce updates one association strength.

An ungraded observation moves the strength toward one by the remaining
gap over the next count. A graded observation splits the current strength
and the feedback across the same unit mass: positive feedback reinforces,
negative feedback inhibits, and zero feedback leaves the strength unchanged.
*/
type Reinforce struct {
	*core.PrimitiveError
}

func NewReinforce() *Reinforce {
	return &Reinforce{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Reinforce) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [4]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 4 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 4 {
			op.Error(core.ErrShape)
			return
		}

		probability := values[0]
		count := values[1]
		feedback := values[2]
		graded := values[3]

		if graded == 0 {
			probability += (core.Unit - probability) / (count + core.Unit)
		}

		if graded != 0 {
			probability /= core.Unit + math.Abs(feedback)

			if feedback > 0 {
				probability += feedback / (core.Unit + feedback)
			}
		}

		for value := range data.NewValue(probability).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}

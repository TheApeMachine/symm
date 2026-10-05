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

			var numbers data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(data.NewMap(
				"probability", "probability",
				"count", "count",
				"feedback", "feedback",
				"graded", "graded",
			))) {
				numbers = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			probability, probabilityOK := numbers.Values["probability"]
			count, countOK := numbers.Values["count"]
			feedback, feedbackOK := numbers.Values["feedback"]
			graded, gradedOK := numbers.Values["graded"]

			if !probabilityOK || !countOK || !feedbackOK || !gradedOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if graded == 0 {
				probability += (core.Unit - probability) / (count + core.Unit)
			}

			if graded != 0 {
				probability /= core.Unit + math.Abs(feedback)

				if feedback > 0 {
					probability += feedback / (core.Unit + feedback)
				}
			}

			issued := data.NewOutputMap()
			issued.Values["probability"] = probability

			for range adapter.Next(data.NewValue(issued)) {
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

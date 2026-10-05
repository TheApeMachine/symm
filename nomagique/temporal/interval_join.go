package temporal

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
IntervalJoin evaluates overlap and advance conditions between ordered interval bounds.
Touching endpoints do not overlap.
*/
type IntervalJoin struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewIntervalJoin() *IntervalJoin {
	output := data.NewOutputMap()
	output.Values["overlap"] = 0
	output.Values["advance_left"] = 0
	output.Values["advance_right"] = 0
	output.Values["from"] = 0
	output.Values["to"] = 0

	return &IntervalJoin{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"left_from", "left_from",
			"left_to", "left_to",
			"right_from", "right_from",
			"right_to", "right_to",
		),
		output: output,
	}
}

func (op *IntervalJoin) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			leftFrom, lfOK := values.Values["left_from"]
			leftTo, ltOK := values.Values["left_to"]
			rightFrom, rfOK := values.Values["right_from"]
			rightTo, rtOK := values.Values["right_to"]

			if !lfOK || !ltOK || !rfOK || !rtOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if leftFrom >= leftTo || rightFrom >= rightTo {
				op.Error(core.ErrDomain)
				return
			}

			overlap := 0.0
			joinedFrom := 0.0
			joinedTo := 0.0

			if leftFrom < rightTo && rightFrom < leftTo {
				overlap = 1.0
				joinedFrom = math.Max(leftFrom, rightFrom)
				joinedTo = math.Min(leftTo, rightTo)
			}

			advanceLeft := 0.0

			if leftTo <= rightTo {
				advanceLeft = 1.0
			}

			advanceRight := 0.0

			if rightTo <= leftTo {
				advanceRight = 1.0
			}

			op.output.Values["overlap"] = overlap
			op.output.Values["advance_left"] = advanceLeft
			op.output.Values["advance_right"] = advanceRight
			op.output.Values["from"] = joinedFrom
			op.output.Values["to"] = joinedTo

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

package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
IntervalOverlap is (left.from < right.to) AND (right.from < left.to).
Endpoints touching at a single instant do not overlap for (from,to] spans.
*/
type IntervalOverlap struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewIntervalOverlap() *IntervalOverlap {
	output := data.NewOutputMap()
	output.Values["overlap"] = 0

	return &IntervalOverlap{
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

func (op *IntervalOverlap) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			overlap := 0.0

			if leftFrom < rightTo && rightFrom < leftTo {
				overlap = 1.0
			}

			op.output.Values["overlap"] = overlap

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

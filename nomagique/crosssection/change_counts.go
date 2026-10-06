package crosssection

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ChangeCounts takes the sign census of the member changes held by the shared
member store and publishes the sign counts, the valid member count, and the
signed fraction (positive - negative) / valid. The member with the largest
absolute change is published as the text "extreme_key".
*/
type ChangeCounts struct {
	*core.PrimitiveError
	members core.Primitive
	output  data.Map[float64]
	text    data.Map[string]
}

func NewChangeCounts(members core.Primitive) *ChangeCounts {
	return &ChangeCounts{
		PrimitiveError: core.NewPrimitiveError(),
		members:        members,
		output:         data.NewOutputMap(),
		text:           data.NewTextMap(),
	}
}

func (op *ChangeCounts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			var snapshot map[string]float64

			for pointer := range op.members.Next(data.NewValue(map[string]float64{})) {
				snapshot = *(*map[string]float64)(pointer)
			}

			if err := op.members.Error(); err != nil {
				op.Error(err)
				return
			}

			positive, negative, zero := 0.0, 0.0, 0.0
			extremeKey := ""
			extreme := 0.0

			for member, change := range snapshot {
				magnitude := math.Abs(change)

				if extremeKey == "" || magnitude > extreme ||
					(magnitude == extreme && member < extremeKey) {
					extreme = magnitude
					extremeKey = member
				}

				switch {
				case change > 0:
					positive++
				case change < 0:
					negative++
				default:
					zero++
				}
			}

			valid := positive + negative + zero

			clear(op.output.Values)
			op.output.Values["valid_member_count"] = valid
			op.output.Values["positive_count"] = positive
			op.output.Values["negative_count"] = negative
			op.output.Values["zero_count"] = zero

			if valid > 0 {
				op.output.Values["signed_fraction"] = (positive - negative) / valid
			}

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if extremeKey != "" {
				op.text.Values["extreme_key"] = extremeKey

				for range adapter.Next(data.NewValue(op.text)) {
				}
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

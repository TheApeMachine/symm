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
member store and yields valid_member_count, positive_count, negative_count,
zero_count, and signed_fraction.
*/
type ChangeCounts struct {
	*core.PrimitiveError
	members    *MemberStore
	extremeKey string
}

func NewChangeCounts(members *MemberStore) *ChangeCounts {
	return &ChangeCounts{
		PrimitiveError: core.NewPrimitiveError(),
		members:        members,
	}
}

func (op *ChangeCounts) ExtremeKey() string {
	return op.extremeKey
}

func (op *ChangeCounts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		compute := func() bool {
			snapshot := op.members.Snapshot()
			positive, negative, zero := 0.0, 0.0, 0.0
			extremeKey := ""
			extreme := 0.0

			for member, change := range snapshot {
				magnitude := math.Abs(change)

				if extremeKey == "" || magnitude > extreme || (magnitude == extreme && member < extremeKey) {
					extreme = magnitude
					extremeKey = member
				}

				if change > 0 {
					positive++
				}

				if change < 0 {
					negative++
				}

				if change == 0 {
					zero++
				}
			}

			op.extremeKey = extremeKey
			valid := positive + negative + zero
			signedFraction := 0.0

			if valid > 0 {
				signedFraction = (positive - negative) / valid
			}

			for value := range data.NewValue(valid, positive, negative, zero, signedFraction).Next(nil) {
				if !yield(value) {
					return false
				}
			}

			return true
		}

		if in == nil {
			compute()
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if !compute() {
				return
			}
		}
	}
}

package crosssection

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ChangeMedian reduces the member changes held by the shared member store to
their median and yields signed_median. An empty cross-section publishes nothing.
*/
type ChangeMedian struct {
	*core.PrimitiveError
	members *MemberStore
}

func NewChangeMedian(members *MemberStore) *ChangeMedian {
	return &ChangeMedian{
		PrimitiveError: core.NewPrimitiveError(),
		members:        members,
	}
}

func (op *ChangeMedian) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		compute := func() bool {
			snapshot := op.members.Snapshot()

			if len(snapshot) == 0 {
				return true
			}

			changes := make([]float64, 0, len(snapshot))

			for _, change := range snapshot {
				changes = append(changes, change)
			}

			slices.Sort(changes)
			count := len(changes)
			median := (changes[(count-1)/2] + changes[count/2]) / 2.0

			for value := range data.NewValue(median).Next(nil) {
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

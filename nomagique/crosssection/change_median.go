package crosssection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
ChangeMedian reduces the member changes held by the shared member store to
their median and publishes it as "signed_median". An empty cross-section
publishes nothing and passes the arrival through.
*/
type ChangeMedian struct {
	*core.PrimitiveError
	members core.Primitive
	median  core.Primitive
	output  data.Map[float64]
}

func NewChangeMedian(members core.Primitive) *ChangeMedian {
	return &ChangeMedian{
		PrimitiveError: core.NewPrimitiveError(),
		members:        members,
		median:         statistic.NewMedian(),
		output:         data.NewOutputMap(),
	}
}

func (op *ChangeMedian) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if len(snapshot) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			changes := make([]float64, 0, len(snapshot))

			for _, change := range snapshot {
				changes = append(changes, change)
			}

			for pointer := range op.median.Next(data.NewValue(changes...)) {
				op.output.Values["signed_median"] = *(*float64)(pointer)
			}

			if err := op.median.Error(); err != nil {
				op.Error(err)
				return
			}

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

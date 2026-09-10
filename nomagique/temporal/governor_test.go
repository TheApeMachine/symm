package temporal

import (
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

type collectionMean struct {
	core.Base[[]float64, float64]
}

func (op *collectionMean) Next(
	in iter.Seq[core.Primitive[[]float64, []float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			values := arriving.Read()
			var sum float64

			for _, value := range values {
				sum += value
			}

			if !yield(op.Carrier(sum / float64(len(values)))) {
				return
			}
		}
	}
}

func TestGovernorNext(t *testing.T) {
	Convey("Governor reduces a retained tail once two observations exist", t, func() {
		op := NewGovernor(2, &collectionMean{})
		out := tests.CollectSeq(op.Next(transport.Values(1.0, 3.0, 7.0, 9.0)))

		So(out, ShouldResemble, []float64{0, 2, 5, 8})
		So(op.Error(), ShouldBeNil)
	})
}

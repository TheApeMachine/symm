package temporal_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

type collectionMean struct {
	*core.PrimitiveError
	out float64
}

func (op *collectionMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]float64)(arriving)
			var sum float64

			for _, value := range values {
				sum += value
			}

			op.out = sum / float64(len(values))

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func TestGovernorNext(t *testing.T) {
	Convey("Governor reduces a retained tail once two observations exist", t, func() {
		op := temporal.NewGovernor(2, &collectionMean{PrimitiveError: core.NewPrimitiveError()})
		vals := []float64{1.0, 3.0, 7.0, 9.0}
		out := make([]float64, 0, len(vals))

		for _, val := range vals {
			adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))
			issued := data.NewOutputMap()
			issued.Values["value"] = val

			for range adapter.Next(data.NewValue(issued)) {
			}

			So(adapter.Error(), ShouldBeNil)

			for range op.Next(data.NewValue(adapter)) {
			}

			So(op.Error(), ShouldBeNil)

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(data.NewMap("value", "value"))) {
				values = *(*data.Map[float64])(pointer)
			}

			So(adapter.Error(), ShouldBeNil)
			out = append(out, values.Values["value"])
		}

		So(out, ShouldResemble, []float64{0, 2, 5, 8})
	})
}

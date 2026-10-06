package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSumNext(t *testing.T) {
	Convey("Given an adapter-native Sum primitive", t, func() {
		op := NewSum()

		Convey("It publishes the running total and leaves the value untouched", func() {
			total := 0.0

			for _, value := range []float64{2, -3.5, 10, 0.25} {
				adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))
				published := data.NewOutputMap()
				published.Values["value"] = value

				for range adapter.Next(data.NewValue(published)) {
				}

				for range op.Next(data.NewValue(adapter)) {
				}

				So(op.Error(), ShouldBeNil)

				total += value
				read := data.Read[data.Map[float64]](
					adapter.Next(data.NewValue(data.NewMap("value", "value", "sum", "sum"))),
				)

				So(adapter.Error(), ShouldBeNil)
				So(read.Values["sum"], ShouldEqual, total)
				So(read.Values["value"], ShouldEqual, value)
			}
		})

		Convey("Domain aliases route the increment and the total", func() {
			shared := data.NewOutputMap()
			shared.Values["qty"] = 4
			adapter := data.NewAdapter(nil, data.NewState(data.NewMap(
				"value", "qty", "sum", "cumulative_qty",
			), shared))

			for range op.Next(data.NewValue(adapter)) {
			}

			So(op.Error(), ShouldBeNil)
			So(shared.Values["cumulative_qty"], ShouldEqual, 4)
			So(shared.Values["qty"], ShouldEqual, 4)
		})

		Convey("A missing value is ErrNotHeld", func() {
			adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))

			for range op.Next(data.NewValue(adapter)) {
			}

			So(errors.Is(op.Error(), core.ErrNotHeld), ShouldBeTrue)
		})

		Convey("When a nil pointer arrives, it records ErrShape", func() {
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}

			for range op.Next(nilSeq) {
			}

			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}

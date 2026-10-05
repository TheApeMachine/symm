package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestQuantileNext(t *testing.T) {
	Convey("Given a Quantile primitive", t, func() {
		Convey("computes linearly interpolated sample quantile", func() {
			q50 := NewQuantile(0.5)
			out := tests.CollectSeq[float64](q50.Next(tests.SliceToSeq([]float64{1.0, 5.0, 3.0})))

			So(q50.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, 3.0)

			q25 := NewQuantile(0.25)
			out25 := tests.CollectSeq[float64](q25.Next(tests.SliceToSeq([]float64{1.0, 2.0, 3.0, 4.0})))

			So(q25.Error(), ShouldBeNil)
			So(len(out25), ShouldEqual, 1)
			So(out25[0], ShouldAlmostEqual, 1.75, 1e-9)
		})

		Convey("invalid quantile parameter records ErrShape", func() {
			invalid := NewQuantile(1.5)
			out := tests.CollectSeq[float64](invalid.Next(tests.SliceToSeq([]float64{1.0, 2.0})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(invalid.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("empty input records ErrShape", func() {
			op := NewQuantile(0.5)
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			op := NewQuantile(0.5)
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](op.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}

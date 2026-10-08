package temporal_test

import (
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestLogReturns(t *testing.T) {
	Convey("LogReturns yields adjacent log differences", t, func() {
		op := temporal.NewLogReturns()
		prices := []temporal.Price{
			{At: 1000, Value: 100.0},
			{At: 2000, Value: 110.0},
			{At: 3000, Value: 121.0},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			for i := range prices {
				yield(unsafe.Pointer(&prices[i]))
			}
		}
		out := tests.CollectSeq[temporal.LogReturn](op.Next(in))
		So(len(out), ShouldEqual, 2)
		So(out[0].From, ShouldEqual, 1000)
		So(out[0].To, ShouldEqual, 2000)
		So(out[0].Value, ShouldAlmostEqual, math.Log(1.1), 1e-9)
		So(out[1].From, ShouldEqual, 2000)
		So(out[1].To, ShouldEqual, 3000)
		So(out[1].Value, ShouldAlmostEqual, math.Log(1.1), 1e-9)
	})

	Convey("LogReturns rejects non-increasing timestamps", t, func() {
		op := temporal.NewLogReturns()
		prices := []temporal.Price{
			{At: 2000, Value: 100.0},
			{At: 1000, Value: 110.0},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			for i := range prices {
				yield(unsafe.Pointer(&prices[i]))
			}
		}
		out := tests.CollectSeq[temporal.LogReturn](op.Next(in))
		So(len(out), ShouldEqual, 0)
		So(op.Error(), ShouldNotBeNil)
	})
}

package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBaseline(t *testing.T) {
	Convey("A delivery run preserves every observation and prior returned value", t, func() {
		baseline := adaptive.NewBaseline(adaptive.NewWindow())
		output := tests.CollectSeq[adaptive.BaselineReading](baseline.Next(transport.NewValues(1.0, 3.0, 5.0).Next(nil)))
		So(baseline.Error(), ShouldBeNil)
		So(output, ShouldHaveLength, 3)
		So(output[0].Mean, ShouldEqual, 1)
		So(output[2].Mean, ShouldEqual, 3)
		So(output[2].Prior.Mean, ShouldEqual, 2)

		more := tests.CollectSeq[adaptive.BaselineReading](baseline.Next(transport.NewValues(7.0).Next(nil)))
		So(more[0].Mean, ShouldEqual, 4)
	})

	Convey("A baseline starts with span 1 and expands adaptively", t, func() {
		baseline := adaptive.NewBaseline(adaptive.NewWindow())
		readings := tests.CollectSeq[adaptive.BaselineReading](baseline.Next(transport.NewValues(42.0, 44.0).Next(nil)))

		So(baseline.Error(), ShouldBeNil)
		So(readings, ShouldHaveLength, 2)
		So(readings[0].Baseline, ShouldEqual, 42.0)
		So(readings[0].Span, ShouldEqual, 1)
		So(readings[0].HasPrior, ShouldBeFalse)

		So(readings[1].Span, ShouldEqual, 2)
		So(readings[1].HasPrior, ShouldBeTrue)
	})

	Convey("Independent cells retain independent fixed-field state", t, func() {
		first := adaptive.NewBaseline(adaptive.NewWindow())
		second := adaptive.NewBaseline(adaptive.NewWindow())

		values := []float64{1, 3, 5, 7}
		out1 := tests.CollectSeq[adaptive.BaselineReading](first.Next(transport.NewValues(values...).Next(nil)))

		values2 := []float64{101, 103, 105, 107}
		out2 := tests.CollectSeq[adaptive.BaselineReading](second.Next(transport.NewValues(values2...).Next(nil)))

		So(out1[len(out1)-1].Mean, ShouldEqual, 4)
		So(out2[len(out2)-1].Mean, ShouldEqual, 104)
		So(out1[len(out1)-1].Dispersion, ShouldAlmostEqual, out2[len(out2)-1].Dispersion)
		So(out1[len(out1)-1].Residual, ShouldAlmostEqual, out2[len(out2)-1].Residual)
		So(out1[len(out1)-1].Maturity, ShouldEqual, 0.75)
		So(out1[len(out1)-1].Span, ShouldEqual, 4)
	})
}

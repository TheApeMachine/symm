package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBaselineNext(t *testing.T) {
	Convey("A delivery run preserves every observation and prior returned value", t, func() {
		baseline := adaptive.NewBaseline(adaptive.NewWindow())
		output := tests.CollectSeq(baseline.Next(transport.Values(1.0, 3.0, 5.0)))
		So(baseline.Error(), ShouldBeNil)
		So(output, ShouldHaveLength, 3)
		So(output[0].Mean, ShouldEqual, 1)
		So(output[2].Mean, ShouldEqual, 3)
		So(output[2].Prior.Mean, ShouldEqual, 2)

		more := tests.CollectSeq(baseline.Next(transport.Values(7.0)))
		So(more[0].Mean, ShouldEqual, 4)
		So(output[0].Mean, ShouldEqual, 1)
	})
}

func TestBaselineObserve(t *testing.T) {
	Convey("A baseline starts with span 1 and expands adaptively", t, func() {
		baseline := adaptive.NewBaseline(adaptive.NewWindow())
		firstReading := baseline.Observe(42.0)
		So(firstReading.Baseline, ShouldEqual, 42.0)
		So(firstReading.Span, ShouldEqual, 1)
		So(firstReading.HasPrior, ShouldBeFalse)

		secondReading := baseline.Observe(44.0)
		So(secondReading.Span, ShouldEqual, 2)
		So(secondReading.HasPrior, ShouldBeTrue)
	})

	Convey("Independent cells retain independent fixed-field state", t, func() {
		first := adaptive.NewBaseline(adaptive.NewWindow())
		second := adaptive.NewBaseline(adaptive.NewWindow())

		for _, value := range []float64{1, 3, 5, 7} {
			first.Observe(value)
			second.Observe(value + 100)
		}

		So(first.Reading.Mean, ShouldEqual, 4)
		So(second.Reading.Mean, ShouldEqual, 104)
		So(first.Reading.Dispersion, ShouldAlmostEqual, second.Reading.Dispersion)
		So(first.Reading.Residual, ShouldAlmostEqual, second.Reading.Residual)
		So(first.Reading.Maturity, ShouldEqual, 0.75)
		So(first.Reading.Span, ShouldEqual, 4)

		allocations := testing.AllocsPerRun(100, func() { first.Observe(5) })
		So(allocations, ShouldEqual, 0)
	})
}

func BenchmarkBaselineObserve(b *testing.B) {
	baseline := adaptive.NewBaseline(adaptive.NewWindow())
	values := [...]float64{1, 3, 5, 7, -2, -4, -6, -8}
	index := 0
	b.ReportAllocs()

	for b.Loop() {
		baseline.Observe(values[index%len(values)])
		index++
	}
}

func BenchmarkBaselineNext(b *testing.B) {
	baseline := adaptive.NewBaseline(adaptive.NewWindow())
	b.ReportAllocs()

	for b.Loop() {
		if _, err := transport.Evaluate(baseline, transport.Values(3.0)); err != nil {
			b.Fatal(err)
		}
	}
}

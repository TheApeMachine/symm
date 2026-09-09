package adaptive_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestBaselineNext(t *testing.T) {
	Convey("The typed recurrence preserves every reference transition across changing regimes", t, func() {
		tests.CheckCausalResidual(t, adaptive.NewBaseline(adaptive.NewWindow()), true)
	})
	Convey("A delivery run preserves every observation and prior returned value", t, func() {
		baseline := adaptive.NewBaseline(adaptive.NewWindow())
		output := tests.Drain(t, baseline, transport.NewIO(core.From(1.0), core.From(3.0), core.From(5.0)))
		So(output, ShouldHaveLength, 3)
		first := tests.Fields(t, output[0])
		last := tests.Fields(t, output[2])
		So(core.To[float64](first["mean"]), ShouldEqual, 1)
		So(core.To[float64](last["mean"]), ShouldEqual, 3)
		So(core.To[float64](last["prior_mean"]), ShouldEqual, 2)
		tests.Drain(t, baseline, tests.Values(7.0))
		So(core.To[float64](first["mean"]), ShouldEqual, 1)
	})
}

func TestBaselineObserve(t *testing.T) {
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
		// The typed observation path must not box values or create record maps.
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
	input := core.From(3.0)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := transport.Evaluate[map[string]core.Primitive](baseline, input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBaselineNextProjection(b *testing.B) {
	baseline := adaptive.NewBaseline(adaptive.NewWindow())
	input := transport.NewIO(core.From(3.0))
	b.ReportAllocs()
	for b.Loop() {
		value := baseline.Next(input)
		// Different downstream consumers read the same immutable delivery.
		for range 8 {
			fields := core.To[map[string]core.Primitive](value)
			if len(fields) != 20 {
				b.Fatal("incomplete baseline record", len(fields))
			}
		}
		if baseline.Next(input) != nil || baseline.Error() != nil {
			b.Fatal("invalid baseline delivery", baseline.Error())
		}
	}
}

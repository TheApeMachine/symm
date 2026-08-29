package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func slopeObservationForTest(value float64, sec float64) types.Frame {
	input := types.Frame{}
	input.Put(types.SampleValue, value)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, 0)

	return input
}

func TestSlope(t *testing.T) {
	Convey("Given a single observation", t, func() {
		pipeline := types.Pipe(
			temporal.Path(""),
			Slope(""),
		)
		stream := types.NewStream(pipeline, types.Frame{})

		Convey("It should not report a slope with fewer than 2 points", func() {
			output := stream.Step(slopeObservationForTest(100, 1000))
			So(output.Err, ShouldBeNil)

			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)

			// Undefined ≠ zero: the slope slot is absent, not a numeric 0.
			_, hasSlope := output.Get(SymbolSlope)
			So(hasSlope, ShouldBeFalse)
		})
	})

	Convey("Given a constant linear trend of 2 units per second", t, func() {
		pipeline := types.Pipe(
			temporal.Path(""),
			Slope(""),
		)
		stream := types.NewStream(pipeline, types.Frame{})

		first := stream.Step(slopeObservationForTest(100, 1000))
		So(first.Err, ShouldBeNil)

		second := stream.Step(slopeObservationForTest(102, 1001))
		So(second.Err, ShouldBeNil)

		third := stream.Step(slopeObservationForTest(104, 1002))
		So(third.Err, ShouldBeNil)

		Convey("It should fit the exact slope beta = 2 and intercept", func() {
			ready, _ := third.Get(SymbolReady)
			So(ready, ShouldEqual, 1)

			slope, _ := third.Get(SymbolSlope)
			So(slope, ShouldAlmostEqual, 2.0, 1e-6)

			intercept, _ := third.Get(SymbolSlopeIntercept)
			So(intercept, ShouldAlmostEqual, 104.0, 1e-6)
		})
	})

	Convey("Given noisy observations around a linear trend", t, func() {
		pipeline := types.Pipe(
			temporal.Path(""),
			Slope(""),
		)
		stream := types.NewStream(pipeline, types.Frame{})

		stream.Step(slopeObservationForTest(10.0, 1000))
		stream.Step(slopeObservationForTest(12.1, 1001))
		stream.Step(slopeObservationForTest(13.9, 1002))
		stream.Step(slopeObservationForTest(16.2, 1003))
		last := stream.Step(slopeObservationForTest(18.0, 1004))
		So(last.Err, ShouldBeNil)

		Convey("It should report slope variance, residual variance, and SNR", func() {
			slope, _ := last.Get(SymbolSlope)
			So(slope, ShouldAlmostEqual, 2.0, 0.1)

			variance, hasVariance := last.Get(SymbolSlopeVariance)
			So(hasVariance, ShouldBeTrue)
			So(variance, ShouldBeGreaterThan, 0)

			snr, hasSNR := last.Get(SymbolSlopeSNR)
			So(hasSNR, ShouldBeTrue)
			So(snr, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a namespaced series prefix", t, func() {
		prefix := "test_metric"
		pipeline := types.Pipe(
			temporal.Path(prefix),
			Slope(prefix),
		)
		stream := types.NewStream(pipeline, types.Frame{})

		series := temporal.NewSeries(prefix)
		slots := newSlopeSlots(prefix)

		inputOne := types.Frame{}
		inputOne.Put(series.ValueSymbol, 50)
		inputOne.Put(series.SecSymbol, 1000)
		inputOne.Put(series.NsecSymbol, 0)
		stream.Step(inputOne)

		inputTwo := types.Frame{}
		inputTwo.Put(series.ValueSymbol, 60)
		inputTwo.Put(series.SecSymbol, 1002)
		inputTwo.Put(series.NsecSymbol, 0)
		second := stream.Step(inputTwo)

		Convey("It should publish under the namespaced slots", func() {
			slope, hasSlope := second.Get(slots.slope)
			So(hasSlope, ShouldBeTrue)
			So(slope, ShouldAlmostEqual, 5.0, 1e-6)
		})
	})
}

func TestSlopeIrregularGridInvariance(t *testing.T) {
	Convey("Given one linear trajectory d(t) = a + b*t", t, func() {
		const slope = 2.0
		const intercept = 100.0

		// Irregular grids expressed as integral (sec, nsec) event timestamps,
		// since temporal.Path requires integral seconds with normalized nsec.
		type stamp struct{ sec, nsec float64 }

		runGrid := func(grid []stamp) float64 {
			pipeline := types.Pipe(
				temporal.Path(""),
				Slope(""),
			)
			stream := types.NewStream(pipeline, types.Frame{})

			var last float64

			for _, timestamp := range grid {
				elapsed := timestamp.sec + timestamp.nsec*1e-9
				input := types.Frame{}
				input.Put(types.SampleValue, intercept+slope*elapsed)
				input.Put(SymbolUnixSec, timestamp.sec)
				input.Put(SymbolUnixNsec, timestamp.nsec)

				output := stream.Step(input)

				if v, found := output.Get(SymbolSlope); found {
					last = v
				}
			}

			return last
		}

		Convey("the fitted beta equals b on two different irregular grids", func() {
			gridA := []stamp{{1000, 0}, {1001, 700e6}, {1003, 100e6}, {1006, 500e6}, {1009, 900e6}, {1014, 200e6}}
			gridB := []stamp{{1000, 0}, {1004, 900e6}, {1005, 300e6}, {1010, 800e6}, {1012, 100e6}, {1018, 900e6}}

			betaA := runGrid(gridA)
			betaB := runGrid(gridB)

			So(betaA, ShouldAlmostEqual, slope, 1e-6)
			So(betaB, ShouldAlmostEqual, slope, 1e-6)
		})
	})
}

func BenchmarkSlope(b *testing.B) {
	pipeline := types.Pipe(
		temporal.Path(""),
		Slope(""),
	)
	stream := types.NewStream(pipeline, types.Frame{})

	// Seed with history
	for iteration := range 10 {
		_ = stream.Step(slopeObservationForTest(100+float64(iteration)*2, 1000+float64(iteration)))
	}

	input := slopeObservationForTest(122, 1011)
	b.ReportAllocs()

	for b.Loop() {
		_ = stream.Step(input)
	}
}

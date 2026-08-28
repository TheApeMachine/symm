package recurrence

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
observation builds one timestamped input for every named series under distinct
prefixes, plus the required comparison-horizon control fact. Timestamps are
supplied per-series so tests can exercise genuinely asynchronous streams (the
case the prior ordinal-indexed scan could not represent). Span gives the ring
capacity so retention itself does not become a confound.
*/
func observation(series []temporal.Series, values []float64, seconds []int64, capacity float64) types.Frame {
	frame := types.Frame{}
	frame.Put(SymbolHorizon, 5)

	for index, oneSeries := range series {
		frame.Put(oneSeries.ValueSymbol, values[index])
		frame.Put(oneSeries.SecSymbol, float64(seconds[index]))
		frame.Put(oneSeries.NsecSymbol, 0)
		frame.Put(oneSeries.SpanSymbol, capacity)
	}

	return frame
}

/*
synchronousObservation feeds every series the same timestamp, for tests whose
subject is the horizon/distance/percentile math rather than cross-stream
alignment.
*/
func synchronousObservation(series []temporal.Series, values []float64, second int64, capacity float64) types.Frame {
	seconds := make([]int64, len(series))

	for index := range seconds {
		seconds[index] = second
	}

	return observation(series, values, seconds, capacity)
}

func newAnalogueStream(prefixes ...string) (*types.Stream, []temporal.Series) {
	series := make([]temporal.Series, len(prefixes))
	paths := make([]types.Primitive, len(prefixes))

	for index, prefix := range prefixes {
		series[index] = temporal.NewSeries(prefix)
		paths[index] = temporal.Path(prefix)
	}

	pipeline := types.Pipe(types.ForkStrict(paths...), Analogue(prefixes...))

	return types.NewStream(pipeline, types.Frame{}), series
}

func TestAnalogue(t *testing.T) {
	Convey("Given no comparison horizon", t, func() {
		stream, series := newAnalogueStream("dim0", "dim1")

		Convey("an absent horizon leaves output undefined, never a silent scan", func() {
			frame := types.Frame{}
			frame.Put(series[0].ValueSymbol, 1)
			frame.Put(series[0].SecSymbol, 1)
			frame.Put(series[0].NsecSymbol, 0)
			frame.Put(series[1].ValueSymbol, 1)
			frame.Put(series[1].SecSymbol, 1)
			frame.Put(series[1].NsecSymbol, 0)

			output := stream.Step(frame)

			So(output.Err, ShouldBeNil)
			_, found := output.Get(SymbolDistance)
			So(found, ShouldBeFalse)
		})

		Convey("a non-positive horizon surfaces as a pipeline error", func() {
			frame := types.Frame{}
			frame.Put(SymbolHorizon, 0)
			frame.Put(series[0].ValueSymbol, 1)
			frame.Put(series[0].SecSymbol, 1)
			frame.Put(series[0].NsecSymbol, 0)
			frame.Put(series[1].ValueSymbol, 1)
			frame.Put(series[1].SecSymbol, 1)
			frame.Put(series[1].NsecSymbol, 0)

			output := stream.Step(frame)

			So(output.Err, ShouldNotBeNil)
		})
	})

	Convey("Given insufficient retained history for one full horizon", t, func() {
		stream, series := newAnalogueStream("dim0", "dim1")

		for index := int64(0); index < 4; index++ {
			output := stream.Step(synchronousObservation(series, []float64{float64(index), float64(index)}, index, temporal.MaxPathSamples))
			So(output.Err, ShouldBeNil)
		}

		Convey("distance and match count are explicitly undefined", func() {
			state := stream.Project()
			_, distanceFound := state.Get(SymbolDistance)
			_, countFound := state.Get(SymbolMatchCount)

			So(distanceFound, ShouldBeFalse)
			So(countFound, ShouldBeFalse)
		})
	})

	Convey("Given enough retained history", t, func() {
		stream, series := newAnalogueStream("dim0", "dim1")

		for index := int64(0); index < 25; index++ {
			stream.Step(synchronousObservation(series, []float64{float64(index % 5), float64(index % 5)}, index, temporal.MaxPathSamples))
		}

		Convey("distance becomes defined and match count grows with retained history", func() {
			state := stream.Project()
			distance, distanceFound := state.Get(SymbolDistance)
			matchCount, countFound := state.Get(SymbolMatchCount)

			So(distanceFound, ShouldBeTrue)
			So(countFound, ShouldBeTrue)
			So(matchCount, ShouldBeGreaterThanOrEqualTo, 2)
			So(distance, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("maturity follows effective support and exceeds the fixed-two-window ceiling", func() {
			state := stream.Project()
			matchCount, countFound := state.Get(SymbolMatchCount)
			So(countFound, ShouldBeTrue)

			maturity, maturityFound := state.Get(SymbolMaturity)
			So(maturityFound, ShouldBeTrue)

			expected := 0.0

			if matchCount > 1 {
				expected = 1 - 1/matchCount
			}

			So(maturity, ShouldEqual, expected)

			if matchCount > 2 {
				So(maturity, ShouldBeGreaterThan, 0.5)
			}
		})

		Convey("the nearest match always precedes the query window, never overlapping it", func() {
			state := stream.Project()
			horizonValue, found := state.Get(SymbolQueryLength)
			So(found, ShouldBeTrue)

			matchFromSec, found := state.Get(SymbolMatchFromSec)
			So(found, ShouldBeTrue)

			// now = 24, Q = 5, so the query occupies [19, 24]. The nearest
			// match must start strictly before 19.
			So(int(matchFromSec), ShouldBeLessThan, 24-int(horizonValue))
		})
	})

	Convey("Given two asynchronous streams", t, func() {
		Convey("they are aligned by wall clock and never zero-filled", func() {
			asyncStream, asyncSeries := newAnalogueStream("async0", "async1")

			// dim0 observes every second; dim1 observes only every fifth
			// second, starting at t=0. Its value holds across quiet gaps, and
			// before its first observation the joint window is not compared at
			// all (no fabricated zero).
			for index := int64(0); index < 30; index++ {
				output := asyncStream.Step(observation(
					asyncSeries,
					[]float64{float64(index), float64(index)},
					[]int64{index, index - (index % 5)},
					temporal.MaxPathSamples,
				))
				So(output.Err, ShouldBeNil)
			}

			state := asyncStream.Project()
			count, found := state.Get(SymbolMatchCount)

			So(found, ShouldBeTrue)
			So(count, ShouldBeGreaterThanOrEqualTo, 1)
		})
	})

	Convey("Given two windows with different change-point grids", t, func() {
		Convey("distance is exact on the union of their relative change offsets, never +Inf", func() {
			// Two dimensions. The query changes dimension A at offsets 2 and 4
			// and dimension B at offset 3; the candidate changes dimension A at
			// offsets 1 and 4 and dimension B at offset 2. Their grids differ,
			// so a positional comparison would wrongly report +Inf.
			query := []dimensionStep{
				{segments: []stepAt{{0, 0}, {2, 1}, {4, 2}}},
				{segments: []stepAt{{0, 0}, {3, 10}}},
			}
			candidate := []dimensionStep{
				{segments: []stepAt{{0, 0}, {1, 1}, {4, 2}}},
				{segments: []stepAt{{0, 0}, {2, 10}}},
			}

			distance := timeWeightedDistance(query, candidate, 5)

			So(math.IsInf(distance, 1), ShouldBeFalse)
			So(distance, ShouldBeGreaterThanOrEqualTo, 0)

			// Hand-computed union grid over [0,5]: offsets 1,2,3,4.
			// dim A mismatch only on (1,2] where +1 vs 0 -> width 1, squared 1.
			// dim B mismatch on (2,3] where 10 vs 0 -> width 1, squared 100.
			// Total = 1*1 + 100*1 = 101 over duration 5 * 2 dims.
			expected := math.Sqrt(101.0 / (5.0 * 2.0))

			So(distance, ShouldAlmostEqual, expected, 1e-9)
		})
	})

	Convey("Given a repeated synthetic trajectory and a deliberately different one", t, func() {
		Convey("the repeated trajectory reports a closer nearest distance", func() {
			repeatedStream, repeatedSeries := newAnalogueStream("repeat0", "repeat1")

			pattern := []float64{0, 1, 4, 1, 0}

			for index := 0; index < 30; index++ {
				value := pattern[index%len(pattern)]
				output := repeatedStream.Step(synchronousObservation(
					repeatedSeries,
					[]float64{value, value},
					int64(index),
					temporal.MaxPathSamples,
				))
				So(output.Err, ShouldBeNil)
			}

			repeatedState := repeatedStream.Project()
			repeatedDistance, found := repeatedState.Get(SymbolDistance)
			So(found, ShouldBeTrue)

			novelStream, novelSeries := newAnalogueStream("novel0", "novel1")

			for index := 0; index < 30; index++ {
				value := float64(index * index)
				output := novelStream.Step(synchronousObservation(
					novelSeries,
					[]float64{value, value},
					int64(index),
					temporal.MaxPathSamples,
				))
				So(output.Err, ShouldBeNil)
			}

			novelState := novelStream.Project()
			novelDistance, found := novelState.Get(SymbolDistance)
			So(found, ShouldBeTrue)

			So(repeatedDistance, ShouldBeLessThan, novelDistance)
		})

		Convey("the recurrence percentile orders an ordinary trajectory below a deliberately novel one", func() {
			// Flat trajectory: every nearest match is near-perfect, so the
			// percentile (fraction of prior scans closer than today) should be
			// low (familiar) once a baseline exists.
			ordinaryStream, ordinarySeries := newAnalogueStream("ordinary0", "ordinary1")

			for index := 0; index < 40; index++ {
				output := ordinaryStream.Step(synchronousObservation(
					ordinarySeries,
					[]float64{1, 1},
					int64(index),
					temporal.MaxPathSamples,
				))
				So(output.Err, ShouldBeNil)
			}

			ordinaryState := ordinaryStream.Project()
			ordinaryPercentile, found := ordinaryState.Get(SymbolPercentile)
			So(found, ShouldBeTrue)

			// A trajectory whose recent segment is a sharp unique spike: its
			// nearest match is an outlier, so the percentile is high (novel).
			distinctiveStream, distinctiveSeries := newAnalogueStream("distinctive0", "distinctive1")
			distinctiveValues := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, -100, 100, -100, 100, -100, 100, -100, 100, 100, 100, 100, 100, 100, -100, -100, -100, -100}

			for index, value := range distinctiveValues {
				output := distinctiveStream.Step(synchronousObservation(
					distinctiveSeries,
					[]float64{value, value},
					int64(index),
					temporal.MaxPathSamples,
				))
				So(output.Err, ShouldBeNil)
			}

			distinctiveState := distinctiveStream.Project()
			distinctivePercentile, found := distinctiveState.Get(SymbolPercentile)
			So(found, ShouldBeTrue)

			So(ordinaryPercentile, ShouldBeLessThan, distinctivePercentile)
		})
	})

	Convey("Given a genuine pipeline failure", t, func() {
		Convey("it surfaces as Err rather than being silently converted into an absent analogue", func() {
			series := []temporal.Series{temporal.NewSeries("failure0")}
			pipeline := types.Pipe(temporal.Path("failure0"), Analogue("failure0"))
			stream := types.NewStream(pipeline, types.Frame{})

			stream.Step(synchronousObservation(series, []float64{1}, 5, temporal.MaxPathSamples))
			output := stream.Step(synchronousObservation(series, []float64{2}, 1, temporal.MaxPathSamples))

			So(output.Err, ShouldNotBeNil)
		})
	})

	Convey("Given deterministic replay of the same input sequence", t, func() {
		Convey("it produces identical Analogue output", func() {
			replay := func() types.Frame {
				stream, series := newAnalogueStream("replay0", "replay1")

				for index := 0; index < 30; index++ {
					value := float64((index * 7) % 5)
					stream.Step(synchronousObservation(series, []float64{value, -value}, int64(index), temporal.MaxPathSamples))
				}

				return stream.Project()
			}

			first := replay()
			second := replay()

			firstDistance, _ := first.Get(SymbolDistance)
			secondDistance, _ := second.Get(SymbolDistance)
			firstPercentile, firstHas := first.Get(SymbolPercentile)
			secondPercentile, secondHas := second.Get(SymbolPercentile)

			So(firstDistance, ShouldEqual, secondDistance)
			So(firstHas, ShouldEqual, secondHas)
			So(firstPercentile, ShouldEqual, secondPercentile)
		})
	})
}

/*
BenchmarkAnalogue measures the steady-state cost of one Analogue scan against a
fully retained multivariate path, once warm.
*/
func BenchmarkAnalogue(benchmark *testing.B) {
	stream, series := newAnalogueStream("bench0", "bench1", "bench2")

	for index := 0; index < 30; index++ {
		value := float64(index)
		stream.Step(synchronousObservation(series, []float64{value, -value, value * value}, int64(index), temporal.MaxPathSamples))
	}

	input := synchronousObservation(series, []float64{1, -1, 1}, 100, temporal.MaxPathSamples)

	benchmark.ReportAllocs()
	

	for benchmark.Loop() {
		_ = stream.Step(input)
	}
}

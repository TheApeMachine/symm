package recurrence

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
observation builds one timestamped input for every named series, all sharing
one event, with an explicit Span so retention capacity does not itself become
a confound while proving the comparison-horizon math.
*/
func observation(series []temporal.Series, values []float64, second int64, capacity float64) types.Frame {
	frame := types.Frame{}

	for index, oneSeries := range series {
		frame.Put(oneSeries.ValueSymbol, values[index])
		frame.Put(oneSeries.SecSymbol, float64(second))
		frame.Put(oneSeries.NsecSymbol, 0)
		frame.Put(oneSeries.SpanSymbol, capacity)
	}

	return frame
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
	Convey("Given insufficient joint history", t, func() {
		stream, series := newAnalogueStream("dim0", "dim1")

		Convey("no observations at all leaves distance explicitly undefined", func() {
			state := stream.Project()
			_, found := state.Get(SymbolDistance)

			So(found, ShouldBeFalse)
		})

		Convey("fewer than five joint observations leave distance explicitly undefined, never a fabricated zero", func() {
			for index := int64(0); index < 4; index++ {
				output := stream.Step(observation(series, []float64{float64(index), float64(index)}, index, temporal.MaxPathSamples))
				So(output.Err, ShouldBeNil)
			}

			state := stream.Project()
			_, distanceFound := state.Get(SymbolDistance)
			_, percentileFound := state.Get(SymbolPercentile)
			_, countFound := state.Get(SymbolMatchCount)

			So(distanceFound, ShouldBeFalse)
			So(percentileFound, ShouldBeFalse)
			So(countFound, ShouldBeFalse)
		})
	})

	Convey("Given exactly the minimum joint history for one valid comparison", t, func() {
		stream, series := newAnalogueStream("dim0", "dim1")

		for index := int64(0); index < 5; index++ {
			output := stream.Step(observation(series, []float64{float64(index), float64(index)}, index, temporal.MaxPathSamples))
			So(output.Err, ShouldBeNil)
		}

		Convey("distance becomes explicitly defined, and its value is honest even when exactly zero", func() {
			state := stream.Project()
			distance, distanceFound := state.Get(SymbolDistance)
			matchCount, matchCountFound := state.Get(SymbolMatchCount)

			So(distanceFound, ShouldBeTrue)
			So(matchCountFound, ShouldBeTrue)
			So(matchCount, ShouldEqual, 1)
			// A defined distance exactly 0 must be distinguishable from
			// "never computed": Get already proved defined above, so the
			// numeric value can be trusted, including a genuine 0.
			So(distance, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("the query cannot match itself: the nearest match start always precedes the query start", func() {
			state := stream.Project()
			queryLengthValue, found := state.Get(SymbolQueryLength)
			So(found, ShouldBeTrue)
			queryLength := int(queryLengthValue)

			matchFromSec, found := state.Get(SymbolMatchFromSec)
			So(found, ShouldBeTrue)

			// The query occupies the most recent queryLength observations
			// (timestamps sampleCount-queryLength .. sampleCount-1 == 3..4
			// here). The reported match must start strictly before that
			// window's own start, proving the scan never compared the
			// current segment against itself.
			So(int(matchFromSec), ShouldBeLessThan, 5-queryLength)
		})
	})

	Convey("Given a repeated synthetic trajectory and a deliberately different one", t, func() {
		Convey("the repeated trajectory reports a closer nearest distance", func() {
			repeatedStream, repeatedSeries := newAnalogueStream("repeat0", "repeat1")

			// A trajectory that repeats an earlier pattern: values cycle with
			// period 5, so the most recent segment is close to a segment
			// exactly one period earlier.
			pattern := []float64{0, 1, 4, 1, 0}

			for index := 0; index < 15; index++ {
				value := pattern[index%len(pattern)]
				output := repeatedStream.Step(observation(
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

			// A monotonically diverging trajectory: no segment resembles any
			// other, so the nearest distance should be larger.
			for index := 0; index < 15; index++ {
				value := float64(index * index)
				output := novelStream.Step(observation(
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

		Convey("recurrence percentile orders an ordinary trajectory below a deliberately novel one", func() {
			// A flat/near-constant trajectory: every window resembles every
			// other window, so the nearest match is not distinctive relative
			// to the other candidates scanned — a high percentile (most
			// candidates are about as close as the nearest one).
			ordinaryStream, ordinarySeries := newAnalogueStream("ordinary0", "ordinary1")

			for index := 0; index < 15; index++ {
				output := ordinaryStream.Step(observation(
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

			// A trajectory whose most recent segment is a sharp, unique spike
			// unlike any earlier segment: the nearest match stands out from
			// the other candidates, so its percentile rank among them is low.
			distinctiveStream, distinctiveSeries := newAnalogueStream("distinctive0", "distinctive1")
			distinctiveValues := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, -100, 100, -100, 100}

			for index, value := range distinctiveValues {
				output := distinctiveStream.Step(observation(
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

			So(distinctivePercentile, ShouldBeLessThanOrEqualTo, ordinaryPercentile)
		})
	})

	Convey("Given a genuine pipeline failure", t, func() {
		Convey("it surfaces as Err rather than being silently converted into an absent analogue", func() {
			series := []temporal.Series{temporal.NewSeries("failure0")}
			pipeline := types.Pipe(temporal.Path("failure0"), Analogue("failure0"))
			stream := types.NewStream(pipeline, types.Frame{})

			stream.Step(observation(series, []float64{1}, 5, temporal.MaxPathSamples))
			// A regressed event time is a genuine defect in the underlying
			// Path retention, which Analogue's own Pipe must propagate, not
			// swallow into "no analogue".
			output := stream.Step(observation(series, []float64{2}, 1, temporal.MaxPathSamples))

			So(output.Err, ShouldNotBeNil)
		})
	})

	Convey("Given deterministic replay of the same input sequence", t, func() {
		Convey("it produces identical Analogue output", func() {
			replay := func() types.Frame {
				stream, series := newAnalogueStream("replay0", "replay1")

				for index := 0; index < 15; index++ {
					value := float64((index * 7) % 5)
					stream.Step(observation(series, []float64{value, -value}, int64(index), temporal.MaxPathSamples))
				}

				return stream.Project()
			}

			first := replay()
			second := replay()

			firstDistance, _ := first.Get(SymbolDistance)
			secondDistance, _ := second.Get(SymbolDistance)
			firstPercentile, _ := first.Get(SymbolPercentile)
			secondPercentile, _ := second.Get(SymbolPercentile)

			So(firstDistance, ShouldEqual, secondDistance)
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

	for index := 0; index < 15; index++ {
		value := float64(index)
		stream.Step(observation(series, []float64{value, -value, value * value}, int64(index), temporal.MaxPathSamples))
	}

	input := observation(series, []float64{1, -1, 1}, 100, temporal.MaxPathSamples)

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for index := 0; index < benchmark.N; index++ {
		_ = stream.Step(input)
	}
}

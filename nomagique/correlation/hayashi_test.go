package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestHayashi(t *testing.T) {
	Convey("Given asynchronously sampled proportional paths", t, func() {
		left := hayashiPath([]int64{0, 1_000_000_000, 2_000_000_000, 3_000_000_000},
			[]float64{100, 110, 121, 133.1})
		right := hayashiPath([]int64{1, 1_000_000_001, 2_000_000_001, 3_000_000_001},
			[]float64{50, 55, 60.5, 66.55})
		output := Hayashi("previous", "current")(pairPaths(left, right))

		Convey("It should correlate every overlapping return interval", func() {
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
			So(math.Abs(output.MustGet(SymbolCorrelation)-1), ShouldBeLessThan, 1e-9)
			So(output.MustGet(SymbolSupport), ShouldEqual, 5.0)
		})
	})
}

/*
pairPaths relocates two retained default-series paths into one frame under the
previous and current series prefixes, the bivariate convention.
*/
func pairPaths(left types.Frame, right types.Frame) types.Frame {
	paired := types.Frame{}
	temporal.NewSeries("previous").CopyFrom(&paired, &left)
	temporal.NewSeries("current").CopyFrom(&paired, &right)

	return paired
}

func hayashiPath(timestamps []int64, values []float64) types.Frame {
	stream := types.NewStream(temporal.Path(""), types.Frame{})

	for index, timestamp := range timestamps {
		input := types.Frame{}
		input.Put(types.SampleValue, values[index])
		input.Put(temporal.SymbolUnixSec, float64(timestamp/1_000_000_000))
		input.Put(temporal.SymbolUnixNsec, float64(timestamp%1_000_000_000))
		input.Put(nmtypes.Span, float64(len(timestamps)))
		output := stream.Step(input)

		if output.Err != nil {
			panic(output.Err)
		}
	}

	return stream.Project()
}

func BenchmarkHayashi(benchmark *testing.B) {
	timestamps := make([]int64, temporal.MaxPathSamples)
	leftValues := make([]float64, temporal.MaxPathSamples)
	rightValues := make([]float64, temporal.MaxPathSamples)

	for index := range temporal.MaxPathSamples {
		timestamps[index] = int64(index) * 1_000_000_000
		leftValues[index] = 100 + float64(index)
		rightValues[index] = 200 + float64(index)
	}

	left := hayashiPath(timestamps, leftValues)
	right := hayashiPath(timestamps, rightValues)
	paired := pairPaths(left, right)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = Hayashi("previous", "current")(paired)
	}
}

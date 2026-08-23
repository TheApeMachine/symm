package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestCrossLag(t *testing.T) {
	Convey("Given a follower carrying the anchor's prior return sequence", t, func() {
		anchor, follower := delayedCrossLagPaths(32)

		_, output, err := CrossLag()(anchor, follower)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolLeadLagReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolLagReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolLagBars), ShouldEqual, 1.0)
		So(output.MustGet(SymbolLagCorrelation), ShouldBeGreaterThan, 0.0)
	})
}

func delayedCrossLagPaths(sampleCount int) (types.Frame, types.Frame) {
	anchorValues := make([]float64, sampleCount)
	followerValues := make([]float64, sampleCount)
	anchorValues[0] = 100
	followerValues[0] = 100
	previousReturn := 0.0

	for index := 1; index < sampleCount; index++ {
		currentReturn := math.Sin(float64(index*index)) / 20
		anchorValues[index] = anchorValues[index-1] * math.Exp(currentReturn)
		followerValues[index] = followerValues[index-1] * math.Exp(previousReturn)
		previousReturn = currentReturn
	}

	return crossLagPath(anchorValues, 0), crossLagPath(followerValues, 0)
}

func crossLagPath(values []float64, offset int64) types.Frame {
	timestamps := make([]int64, len(values))

	for index := range values {
		timestamps[index] = int64(index)*1_000_000_000 + offset
	}

	return hayashiEquationPath(timestamps, values)
}

func hayashiEquationPath(timestamps []int64, values []float64) types.Frame {
	path := types.NewStream(temporal.Path, types.Frame{})

	for index, timestamp := range timestamps {
		input := types.Frame{}
		input.Put(types.SampleValue, values[index])
		input.Put(temporal.SymbolUnixSec, float64(timestamp/1_000_000_000))
		input.Put(temporal.SymbolUnixNsec, float64(timestamp%1_000_000_000))
		input.Put(nmtypes.Span, float64(len(timestamps)))
		_, err := path.Step(input)

		if err != nil {
			panic(err)
		}
	}

	return path.Project()
}

func BenchmarkCrossLag(benchmark *testing.B) {
	anchor, follower := delayedCrossLagPaths(32)
	algorithm := CrossLag()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = algorithm(anchor, follower)
	}
}

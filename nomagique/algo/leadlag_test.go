package algo

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestLeadLag(t *testing.T) {
	Convey("Given a follower carrying the anchor's prior returns", t, func() {
		anchor, follower := leadLagTestPaths(32)
		_, output, err := LeadLag()(anchor, follower)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolInefficiency), ShouldBeGreaterThan, 0.0)
		So(output.MustGet(SymbolLeadLagStrength), ShouldBeGreaterThan, 0.0)
		So(output.MustGet(SymbolLagDirection), ShouldEqual, 1.0)
	})
}

func leadLagTestPaths(sampleCount int) (types.Frame, types.Frame) {
	anchor := nomagique.NewStream(temporal.Path, types.Frame{})
	follower := nomagique.NewStream(temporal.Path, types.Frame{})
	anchorValue := 100.0
	followerValue := 100.0
	previousReturn := 0.0

	for index := range sampleCount {
		if index > 0 {
			currentReturn := math.Sin(float64(index*index)) / 20
			anchorValue *= math.Exp(currentReturn)
			followerValue *= math.Exp(previousReturn)
			previousReturn = currentReturn
		}

		_, err := anchor.Step(leadLagObservation(anchorValue, index, sampleCount))

		if err != nil {
			panic(err)
		}

		_, err = follower.Step(leadLagObservation(followerValue, index, sampleCount))

		if err != nil {
			panic(err)
		}
	}

	return anchor.Project(), follower.Project()
}

func leadLagObservation(value float64, index int, capacity int) types.Frame {
	input := types.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(temporal.SymbolUnixSec, float64(index))
	input.Put(temporal.SymbolUnixNsec, 0)
	input.Put(nmtypes.Span, float64(capacity))

	return input
}

func BenchmarkLeadLag(benchmark *testing.B) {
	anchor, follower := leadLagTestPaths(32)
	algorithm := LeadLag()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = algorithm(anchor, follower)
	}
}

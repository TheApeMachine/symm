package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestCohort(t *testing.T) {
	Convey("Given ready pair estimates with unequal support", t, func() {
		state := cohortPair(0.5, 2, 4, 1)
		Cohort(&state)
		So(state.Err, ShouldBeNil)
		merged := state
		merged.Merge(cohortPair(-0.25, 4, 4, 2))
		Cohort(&merged)

		So(merged.Err, ShouldBeNil)
		So(merged.MustGet(SymbolTotalSupport), ShouldEqual, 6.0)
		So(merged.MustGet(SymbolWeightedSigned), ShouldEqual, 0.0)
		So(merged.MustGet(SymbolWeightedAbsolute), ShouldEqual, 2.0)
		So(merged.MustGet(SymbolWeightedPeerEnergy), ShouldEqual, 10.0)
		So(merged.MustGet(SymbolPeerCount), ShouldEqual, 2.0)
	})

	Convey("Given an unsupported pair", t, func() {
		output := cohortPair(1, 1, 1, 1)
		Cohort(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 0.0)
	})
}

func cohortPair(
	correlation float64,
	support float64,
	leftVariance float64,
	rightVariance float64,
) types.Frame {
	input := types.Frame{}
	input.Put(SymbolReady, 1)
	input.Put(SymbolCorrelation, correlation)
	input.Put(SymbolSupport, support)
	input.Put(SymbolLeftVariance, leftVariance)
	input.Put(SymbolRightVariance, rightVariance)

	return input
}

func BenchmarkCohort(benchmark *testing.B) {
	input := cohortPair(0.5, 4, 2, 1)
	state := types.Frame{}
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		merged := state
		merged.Merge(input)
		Cohort(&merged)
	}
}

package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestCohort(t *testing.T) {
	Convey("Given ready pair estimates with unequal support", t, func() {
		state, _, err := Cohort(types.Frame{}, cohortPair(0.5, 2, 4, 1))
		So(err, ShouldBeNil)
		state, output, err := Cohort(state, cohortPair(-0.25, 4, 4, 2))

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolTotalSupport), ShouldEqual, 6.0)
		So(output.MustGet(SymbolWeightedSigned), ShouldEqual, 0.0)
		So(output.MustGet(SymbolWeightedAbsolute), ShouldEqual, 2.0)
		So(output.MustGet(SymbolWeightedPeerEnergy), ShouldEqual, 10.0)
		So(output.MustGet(SymbolPeerCount), ShouldEqual, 2.0)
	})

	Convey("Given an unsupported pair", t, func() {
		_, output, err := Cohort(types.Frame{}, cohortPair(1, 1, 1, 1))

		So(err, ShouldBeNil)
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
		_, _, _ = Cohort(state, input)
	}
}

package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestScores(t *testing.T) {
	Convey("Given a coherent cohort with focal excess energy", t, func() {
		state := types.Frame{}
		state.Put(SymbolReady, 1)
		state.Put(SymbolTotalSupport, 4)
		state.Put(SymbolWeightedSigned, 3)
		state.Put(SymbolWeightedAbsolute, 3)
		state.Put(SymbolWeightedPeerEnergy, 8)
		state.Put(SymbolFocalEnergy, 4)

		_, output, err := Scores()(state, state)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolCohortCorrelation), ShouldEqual, 0.75)
		So(output.MustGet(SymbolSignedCorrelation), ShouldEqual, 0.75)
		So(output.MustGet(SymbolRelativeEnergy), ShouldEqual, 2.0)
		So(output.MustGet(SymbolHerd), ShouldEqual, 0.375)
		So(output.MustGet(SymbolAlpha), ShouldEqual, 1.0/3.0)
		So(output.MustGet(SymbolNoise), ShouldEqual, 0.25)
		So(output.MustGet(SymbolStress), ShouldEqual, 0.0)
	})
}

func BenchmarkScores(benchmark *testing.B) {
	state := types.Frame{}
	state.Put(SymbolReady, 1)
	state.Put(SymbolTotalSupport, 4)
	state.Put(SymbolWeightedSigned, 3)
	state.Put(SymbolWeightedAbsolute, 3)
	state.Put(SymbolWeightedPeerEnergy, 8)
	state.Put(SymbolFocalEnergy, 4)
	algorithm := Scores()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = algorithm(state, state)
	}
}

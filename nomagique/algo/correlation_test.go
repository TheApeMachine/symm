package algo

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestCorrelation(t *testing.T) {
	Convey("Given a coherent cohort with focal excess energy", t, func() {
		state := types.Frame{}
		state.Put(nmcorrelation.SymbolReady, 1)
		state.Put(nmcorrelation.SymbolTotalSupport, 4)
		state.Put(nmcorrelation.SymbolWeightedSigned, 3)
		state.Put(nmcorrelation.SymbolWeightedAbsolute, 3)
		state.Put(nmcorrelation.SymbolWeightedPeerEnergy, 8)
		state.Put(nmcorrelation.SymbolFocalEnergy, 4)

		_, output, err := Correlation()(state, state)

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

func BenchmarkCorrelation(benchmark *testing.B) {
	state := types.Frame{}
	state.Put(nmcorrelation.SymbolReady, 1)
	state.Put(nmcorrelation.SymbolTotalSupport, 4)
	state.Put(nmcorrelation.SymbolWeightedSigned, 3)
	state.Put(nmcorrelation.SymbolWeightedAbsolute, 3)
	state.Put(nmcorrelation.SymbolWeightedPeerEnergy, 8)
	state.Put(nmcorrelation.SymbolFocalEnergy, 4)
	algorithm := Correlation()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = algorithm(state, state)
	}
}

package resonance

import (
	"testing"

	"github.com/theapemachine/nomagique/data"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExtractFeaturesStoryAndBoard(t *testing.T) {
	Convey("extractFeatures builds the R00–R14 vector", t, func() {
		solver := NewSolver(t.Context())
		So(solver, ShouldNotBeNil)

		tick := data.NewValue("workspace", "tick", "test")
		tick.Append([]float64{1.1}, "pumpdump", "PEAQ", "USD", "coiled_compression")
		tick.Append([]float64{0.8}, "correlation", "aggregate", "structural_alignment")
		tick.Append([]float64{0.9}, "pumpdump", "aggregate", "board_coiled")
		tick.Append([]float64{0.05}, "liquidity", "aggregate", "market_friction")

		raw, _ := solver.extractFeatures("PEAQ/USD", tick)
		So(len(raw), ShouldEqual, len(Features))

		scorer, _ := solver.standardizers.Load("PEAQ/USD")
		So(scorer, ShouldNotBeNil)
	})
}

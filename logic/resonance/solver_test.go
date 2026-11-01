package resonance

import (
	"testing"

	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExtractFeaturesStoryAndBoard(t *testing.T) {
	Convey("extractFeatures builds the R00–R14 vector", t, func() {
		solver := NewSolver(t.Context())
		So(solver, ShouldNotBeNil)

		metrics := map[string]map[string]map[string]float64{
			"PEAQ/USD": {
				"pumpdump": {"coiled_compression": 1.1},
			},
			"aggregate": {
				"correlation": {"structural_alignment": 0.8},
				"pumpdump":    {"board_coiled": 0.9},
				"liquidity":   {"market_friction": 0.05},
			},
		}

		raw, _ := solver.extractFeatures("PEAQ/USD", metrics)
		So(len(raw), ShouldEqual, len(Features))

		scorer, _ := solver.standardizers.Load("PEAQ/USD")
		So(scorer, ShouldNotBeNil)
	})
}

func TestStepNeedsEnvelope(t *testing.T) {
	Convey("Step rejects a nil envelope", t, func() {
		solver := NewSolver(t.Context())
		So(solver.Step(nil), ShouldBeNil)

		envelope := runtime.NewEnvelope(&market.Trade{
			Data: []*market.TradeData{{Symbol: "PEAQ/USD"}},
		})
		So(solver.Step(envelope), ShouldEqual, envelope)
	})
}

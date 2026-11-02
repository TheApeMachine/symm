package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

func TestObserveRequiresFullVector(t *testing.T) {
	Convey("Observe skips incomplete metrics and accepts a full signal vector", t, func() {
		solver := NewSolver(t.Context())
		So(solver, ShouldNotBeNil)

		scorer := newFeatureScorer()

		partial := map[string]map[string]float64{
			"pumpdump": {"compression": 1.1},
		}
		input, ready := scorer.Observe(partial)
		So(ready, ShouldBeFalse)
		So(input, ShouldBeNil)

		full := map[string]map[string]float64{
			"pumpdump":    {"compression": 1.1, "exhaustion": 0.4},
			"correlation": {"herd_score": 0.8, "peak_score": 0.7},
			"cvd":         {"starvation": 0.2},
			"depthflow":   {"thin_score": 0.3, "loaded_score": 0.6},
			"morphology":  {"ask_vacuum": 0.15},
			"hawkes":      {"spectral_radius": 1.1},
			"liquidity":   {"scarcity_score": 0.2, "depth_score": 0.9},
			"toxicity":    {"vacuum_score": 0.25},
			"sentiment":   {"slump_score": 0.1},
			"leadlag":     {"sync_score": 0.6, "decoupled_score": 0.05},
		}

		input, ready = scorer.Observe(full)
		So(len(input), ShouldEqual, len(Features))
		So(ready, ShouldBeFalse)
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

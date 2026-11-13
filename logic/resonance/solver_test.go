package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

func fullSources(shift float64) map[string]map[string]float64 {
	return map[string]map[string]float64{
		"pumpdump":    {"compression": 1.1 + shift, "exhaustion": 0.4 + shift},
		"correlation": {"herd_score": 0.8 + shift, "peak_score": 0.7 + shift},
		"cvd":         {"starvation": 0.2 + shift},
		"depthflow":   {"thin_score": 0.3 + shift, "loaded_score": 0.6 + shift},
		"morphology":  {"ask_vacuum": 0.15 + shift},
		"hawkes":      {"spectral_radius": 1.1 + shift},
		"liquidity":   {"scarcity_score": 0.2 + shift, "depth_score": 0.9 + shift},
		"toxicity":    {"vacuum_score": 0.25 + shift},
		"sentiment":   {"slump_score": 0.1 + shift},
		"leadlag":     {"sync_score": 0.6 + shift, "decoupled_score": 0.05 + shift},
	}
}

func TestObserve(t *testing.T) {
	Convey("Setup", t, func() {
		scorer := newFeatureScorer()

		Convey("When only one source metric arrives", func() {
			input, ready := scorer.Observe(map[string]map[string]float64{
				"pumpdump": {"compression": 1.1},
			})

			Convey("The scorer is not ready", func() {
				So(ready, ShouldBeFalse)
				So(input, ShouldBeNil)
			})
		})

		Convey("When partial ticks arrive across sources", func() {
			_, firstReady := scorer.Observe(map[string]map[string]float64{
				"pumpdump": {"compression": 1.1},
			})
			So(firstReady, ShouldBeFalse)

			parts := []map[string]map[string]float64{
				{"pumpdump": {"exhaustion": 0.4}},
				{"correlation": {"herd_score": 0.8, "peak_score": 0.7}},
				{"cvd": {"starvation": 0.2}},
				{"depthflow": {"thin_score": 0.3, "loaded_score": 0.6}},
				{"morphology": {"ask_vacuum": 0.15}},
				{"hawkes": {"spectral_radius": 1.1}},
				{"liquidity": {"scarcity_score": 0.2, "depth_score": 0.9}},
				{"toxicity": {"vacuum_score": 0.25}},
				{"sentiment": {"slump_score": 0.1}},
				{"leadlag": {"sync_score": 0.6, "decoupled_score": 0.05}},
			}

			var input []float64
			var ready bool

			for _, part := range parts[:len(parts)-1] {
				input, ready = scorer.Observe(part)
				So(ready, ShouldBeFalse)
				So(input, ShouldBeNil)
			}

			input, ready = scorer.Observe(parts[len(parts)-1])

			Convey("The final partial completes the feature vector", func() {
				So(len(input), ShouldEqual, len(Features))
				_ = ready
			})
		})

		Convey("When full source maps shift until ready", func() {
			settled := false
			var input []float64

			for shift := 0.1; shift < 1.0; shift += 0.1 {
				var ready bool
				input, ready = scorer.Observe(fullSources(shift))
				if ready {
					settled = true
					break
				}
			}

			Convey("A complete input vector is produced", func() {
				So(settled, ShouldBeTrue)
				So(len(input), ShouldEqual, len(Features))
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		solver := NewSolver(t.Context())

		Convey("When Step receives a nil envelope", func() {
			out := solver.Step(nil)

			Convey("It returns nil", func() {
				So(out, ShouldBeNil)
			})
		})

		Convey("When Step receives a trade envelope before features settle", func() {
			envelope := runtime.NewEnvelope(&market.Trade{
				Data: []*market.TradeData{{Symbol: "PEAQ/USD"}},
			})
			out := solver.Step(envelope)

			Convey("The same envelope is returned", func() {
				So(out, ShouldEqual, envelope)
			})
		})
	})
}

func TestStepSymbol(t *testing.T) {
	Convey("Setup", t, func() {
		solver := NewSolver(t.Context())
		symbol := "ZEC/USD"

		Convey("When feature maps shift until stepSymbol settles", func() {
			var reading map[string]float64

			for shift := 0.0; shift < 2.0; shift += 0.15 {
				reading = solver.stepSymbol(symbol, fullSources(shift))
				if reading != nil {
					break
				}
			}

			Convey("A resonance reading includes surprise and a forward curve slot", func() {
				So(reading, ShouldNotBeNil)
				So(reading["surprise"], ShouldNotEqual, 0)
				_, hasCurve := reading["forward_curve_0"]
				So(hasCurve, ShouldBeTrue)
			})
		})
	})
}

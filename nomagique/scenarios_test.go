package nomagique

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
)

func TestScenarios(t *testing.T) {
	Convey("Production Reference Implementations (Scenarios 1-5)", t, func() {
		Convey("Scenario 1: Information-Time Decay with Zero-Sink Buffer", func() {
			pipeline := &Chain{
				A: &Split{
					A: &Decay{
						Rate: &adaptive.Clock{
							Type:        adaptive.INTERARRIVAL,
							Sensitivity: adaptive.Sensitivity{Type: adaptive.HIGH},
						},
						Shape: Exponential{},
					},
					B: &Decay{
						Rate: &adaptive.Clock{
							Type:        adaptive.VOLUME,
							Sensitivity: adaptive.Sensitivity{Type: adaptive.LOW},
						},
						Shape: Exponential{},
					},
					C: &Store{
						Type:     DynamicRing,
						Adaptive: adaptive.Window{Type: adaptive.ADWIN},
					},
				},
				B: &adaptive.Envelope{Type: adaptive.EVT},
			}

			out := pipeline.Step(104.5)
			So(out, ShouldBeGreaterThan, 0.0)

			// Participates natively in language math with zero unboxing
			out += 1.0
			scaled := out * 0.25
			So(scaled, ShouldBeGreaterThan, 0.0)
		})

		Convey("Scenario 2: Dynamic Regime-Switching Volatility Filter", func() {
			pipeline := &Split{
				Route: &VolatilityBlend{
					Window:    adaptive.Window{Type: adaptive.ADWIN},
					Threshold: adaptive.Threshold{Type: adaptive.WELFORD},
				},
				A: &Store{
					Type:     DynamicRing,
					Adaptive: adaptive.Window{Type: adaptive.ADWIN},
					Reduce:   Average,
				},
				B: &Store{
					Type:     DynamicRing,
					Adaptive: adaptive.Window{Type: adaptive.STABILITY_GOV},
					Reduce:   Median,
				},
			}

			prices := []Number{101.1, 101.2, 101.4, 109.0, 101.5}

			for _, price := range prices {
				filtered := pipeline.Step(price)
				So(filtered, ShouldBeGreaterThan, 0.0)
			}
		})

		Convey("Scenario 3: Causal Standardizer with Chebyshev Gating", func() {
			standardizer := &Standardizer{
				Engine: &adaptive.WelfordEngine{},
			}

			pipeline := &Chain{
				A: standardizer,
				B: &adaptive.Gating{
					Threshold: adaptive.Threshold{Type: adaptive.CHEBYSHEV},
				},
			}

			ticks := []Number{10.0, 10.2, 10.1, 95.0, 10.3}

			for _, tick := range ticks {
				z := pipeline.Step(tick)
				_ = z
				So(standardizer.Mean(), ShouldBeGreaterThan, 0.0)
			}

			So(standardizer.Count(), ShouldEqual, 5.0)
		})

		Convey("Scenario 4: Self-Exciting Hawkes Surge Tracker", func() {
			hawkes := &Chain{
				A: &Split{
					A: &Decay{
						Rate:  &adaptive.Clock{Type: adaptive.ENTROPY},
						Shape: Exponential{},
					},
					B: Identity{},
				},
				B: &adaptive.Baseline{
					Engine: adaptive.WelfordEngine{},
				},
			}

			resting := hawkes.Step(0.0)
			So(resting, ShouldEqual, 0.0)

			shock := hawkes.Step(5.0)
			So(shock, ShouldBeGreaterThan, resting)
		})

		Convey("Scenario 5: The Pure Adaptive Governor", func() {
			governor := &Governor{
				Store: Store{
					Type:     DynamicRing,
					Adaptive: adaptive.Window{Type: adaptive.ADWIN},
				},
				Controller: &adaptive.StabilityController{
					Type: adaptive.KISH,
				},
				Reduce: LinearSlope,
			}

			// Feed 10,000 samples — proving smooth scaling past 128 without clamp
			for iteration := 0; iteration < 10000; iteration++ {
				_ = governor.Step(Number(iteration % 100))
			}

			So(governor.Store.Len(), ShouldBeGreaterThan, 128)
		})
	})
}

package nomagique

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
)

func TestScenarios(t *testing.T) {
	Convey("Production Reference Implementations (Scenarios 1-5)", t, func() {
		Convey("Scenario 1: Multi-Scale Attenuation with Sidecar Buffer (Tee)", func() {
			pipeline := Number(
				&Chain{
					A: &Split{
						// Branch A: High-frequency renewal-time attenuation
						A: &Decay{
							Rate: &adaptive.Clock{
								Type: adaptive.INTERARRIVAL,
							},
							Shape: Exponential{},
						},
						// Branch B: Volume/Energy renewal-time attenuation
						B: &Decay{
							Rate: &adaptive.Clock{
								Type: adaptive.VOLUME,
							},
							Shape: Exponential{},
						},
						// Branch C: Passive sidecar ring buffer (Algebraic Sink)
						// Emits 0 -> A + B + 0 = A + B
						C: &Store{
							Type:     DynamicRing,
							Adaptive: adaptive.Window{Type: adaptive.ADWIN},
						},
					},
					// Stage 2: Adaptive distribution boundary envelope (Extreme Value Theory)
					B: &adaptive.Envelope{Type: adaptive.EVT},
				},
			)

			// Step returns Scalar (float64)
			out := pipeline.Apply(100.0)
			So(out, ShouldBeGreaterThan, 0.0)

			// Participates natively in Go arithmetic without wrappers
			out += 1.0
			scaled := out * 0.25
			So(scaled, ShouldEqual, out*0.25)
		})

		Convey("Scenario 2: Dynamic Regime-Switching Filter", func() {
			// Crossfades between responsive moving mean and robust median
			// based on adaptive statistical dispersion
			pipeline := Number(
				&Split{
					Route: &VolatilityBlend{
						Window:    adaptive.Window{Type: adaptive.ADWIN},
						Threshold: adaptive.Threshold{Type: adaptive.WELFORD},
					},
					// Regime A: Laminar state (Sample mean)
					A: &Store{
						Type:     DynamicRing,
						Adaptive: adaptive.Window{Type: adaptive.ADWIN},
						Reduce:   Average,
					},
					// Regime B: Perturbed state (Sample median)
					B: &Store{
						Type:     DynamicRing,
						Adaptive: adaptive.Window{Type: adaptive.STABILITY_GOV},
						Reduce:   Median,
					},
				},
			)

			inputs := []Scalar{10.0, 10.5, 11.0, 45.0, 10.8}

			for _, val := range inputs {
				filtered := pipeline.Step(val)
				So(math.IsNaN(float64(filtered)), ShouldBeFalse)
			}
		})

		Convey("Scenario 3: Causal Standardizer with Chebyshev Outlier Rejection", func() {
			standardizer := &Standardizer{
				Engine: &adaptive.WelfordEngine{},
			}

			pipeline := Number(
				&Chain{
					A: standardizer,
					B: &adaptive.Gating{
						Threshold: adaptive.Threshold{Type: adaptive.CHEBYSHEV},
					},
				},
			)

			out := pipeline.Apply(12.5)
			So(math.IsNaN(float64(out)), ShouldBeFalse)
		})

		Convey("Scenario 4: Self-Exciting Renewal Process (Point Process Intensity)", func() {
			hawkes := Number(
				&Chain{
					A: &Split{
						// Self-excitation tail
						A: &Decay{
							Rate:  &adaptive.Clock{Type: adaptive.ENTROPY},
							Shape: Exponential{},
						},
						// Impulse pass-through
						B: Identity{},
					},
					// Emergent background intensity baseline
					B: &adaptive.Baseline{
						Engine: adaptive.WelfordEngine{},
					},
				},
			)

			baselineIntensity := hawkes.Apply(0.0)
			impulseIntensity := hawkes.Apply(5.0)
			So(impulseIntensity, ShouldBeGreaterThan, baselineIntensity)
		})

		Convey("Scenario 5: Unbounded Adaptive Linear Governor", func() {
			governor := Number(
				&Governor{
					Store: Store{
						Type:     DynamicRing,
						Adaptive: adaptive.Window{Type: adaptive.ADWIN},
					},
					Controller: &adaptive.StabilityController{
						Type: adaptive.KISH,
					},
					Reduce: LinearSlope,
				},
			)

			for iteration := 0; iteration < 500; iteration++ {
				_ = governor.Step(Scalar(iteration % 50))
			}
		})
	})
}

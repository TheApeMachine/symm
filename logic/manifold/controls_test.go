package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
)

func TestRuntimeControls(t *testing.T) {
	Convey("Given a Hawkes outcome with positive spectral radius and decay", t, func() {
		config := testPhysicsConfig()
		outcome := testOutcome()

		Convey("It should scale GPE interaction by eta and set damping to beta", func() {
			controls := runtimeControls(config, outcome, outcome.Horizon)

			So(controls.GInteraction, ShouldAlmostEqual, config.GInteraction()*outcome.Fit.SpectralRadius)
			So(controls.EnergyDecay, ShouldEqual, outcome.Fit.Beta)
			So(controls.TopdownPhaseScale, ShouldAlmostEqual, config.CouplingScale()*outcome.Fit.SpectralRadius)
			So(controls.DeltaT, ShouldBeGreaterThan, 0)
			So(controls.Validate(), ShouldBeNil)
		})
	})

	Convey("Given a Hawkes outcome without a fitted kernel", t, func() {
		config := testPhysicsConfig()
		outcome := excitation.Outcome{
			At:        time.Unix(1, 0),
			Horizon:   time.Second,
			Readiness: excitation.Readiness{Observation: true},
			Fit: hawkes.BivariateFit{
				Beta: 2,
			},
		}

		Convey("It should keep the static engine defaults", func() {
			controls := runtimeControls(config, outcome, outcome.Horizon)

			So(controls.GInteraction, ShouldEqual, config.GInteraction())
			So(controls.EnergyDecay, ShouldEqual, config.EnergyDecay())
		})
	})
}

package nomagique_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
TestCompositionIsTheDefinition holds the architecture to its claim: a whole
measurement must be expressible as ONE nested literal, with no intermediate
variables holding node pointers and no wiring performed outside the graph.

If this test needs a local variable to reach a stage's value, the system is an
imperative program wearing a pipeline's clothes.
*/
func TestCompositionIsTheDefinition(t *testing.T) {
	Convey("Given a whole measurement declared as one nested literal", t, func() {
		pipeline := nomagique.Number(
			&types.Chain{
				A: &calculus.Accumulator{},
				B: &equation.CausalResidual{},
				C: &temporal.Velocity{},
				D: &data.Projection{Source: "cvd"},
			},
		)

		Convey("stepping it flows A -> B -> C -> D", func() {
			So(pipeline.Step(500), ShouldNotBeNil)
		})

		Convey("the measurement is read from the pipeline itself", func() {
			pipeline.Step(500)
			pipeline.Step(300)

			measurement := pipeline.Measurement()
			So(measurement, ShouldNotBeNil)
			So(measurement.Source, ShouldEqual, "cvd")

			Convey("carrying every upstream stage's own readings", func() {
				// The accumulator's total: 500 then 500+300.
				So(measurement.Metrics["total"].Raw, ShouldEqual, 800.0)

				// The estimator standardized what the accumulator emitted.
				_, hasBaseline := measurement.Metrics["baseline"]
				_, hasZScore := measurement.Metrics["zscore"]
				So(hasBaseline, ShouldBeTrue)
				So(hasZScore, ShouldBeTrue)
			})

			Convey("and the confidence the estimator itself declared", func() {
				So(measurement.Maturity, ShouldBeGreaterThan, 0)
			})
		})

		Convey("an undefined reading is absent, not published as zero", func() {
			pipeline.Step(500)

			// One observation: the estimator has no prior, so nothing it
			// derives is stateable yet.
			_, hasBaseline := pipeline.Measurement().Metrics["baseline"]
			So(hasBaseline, ShouldBeFalse)
		})
	})

	Convey("Given a composition with no projection", t, func() {
		pipeline := nomagique.Number(&calculus.Accumulator{})

		Convey("there is no measurement to read", func() {
			pipeline.Step(1)
			So(pipeline.Measurement(), ShouldBeNil)
		})
	})

	Convey("Given a projection naming its observation", t, func() {
		at := time.Unix(1_700_000_000, 0).UTC()

		pipeline := nomagique.Number(
			&types.Chain{
				A: &calculus.Accumulator{},
				B: &data.Projection{
					Source: "cvd",
					Identity: func() (string, string, time.Time, time.Time) {
						return "BTC/USD:cvd:1", "BTC/USD", at, at
					},
				},
			},
		)

		Convey("the identity reaches the measurement", func() {
			pipeline.Step(42)

			measurement := pipeline.Measurement()
			So(measurement.ID, ShouldEqual, "BTC/USD:cvd:1")
			So(measurement.Label, ShouldEqual, "BTC/USD")
			So(measurement.At, ShouldEqual, at)
		})
	})
}

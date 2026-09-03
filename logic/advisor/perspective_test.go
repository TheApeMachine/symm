package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

func TestIssuerIssue(t *testing.T) {
	Convey("Given predictive declarations for every competing Class", t, func() {
		solver := NewSolver(t.Context(), "midpoint", predictiveMidpointFeatures())
		conditionMidpointSolver(solver)
		envelope := advisorMeasurementEnvelope(2, 3, true, nil)

		Convey("the unique lean issues every Class's tagged counterfactual predictions", func() {
			So(solver.Step(envelope), ShouldEqual, envelope)
			So(envelope.Perspectives, ShouldHaveLength, 1)

			perspective := envelope.Perspectives[0]
			So(perspective.Symbol, ShouldEqual, "BTC/USD")
			So(perspective.Question, ShouldEqual, types.PerspectiveQuestion("midpoint"))
			So(perspective.Lifecycle, ShouldEqual, types.PerspectiveIssued)
			So(perspective.Lease.From, ShouldEqual, uint64(3))
			So(perspective.Lease.Until, ShouldEqual, uint64(5))
			So(perspective.Predictions, ShouldHaveLength, 4)
			So(perspective.Predictions[0].Class, ShouldEqual, types.PerspectiveState("recovery"))
			So(perspective.Predictions[2].Class, ShouldEqual, types.PerspectiveState("breakdown"))
		})
	})
}

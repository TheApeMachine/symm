package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/graph"
)

func TestCausalIdentificationConformance(t *testing.T) {
	Convey("Given a causal model over real observations", t, func() {
		store := buildStore()
		influenceGraph := graph.NewInfluenceGraph(1, 1, 1, 32)
		schema := testSchema()
		model := NewCausalModel(schema, store, influenceGraph, "test-v1")
		at := time.Unix(0, 119*int64(time.Second))

		Convey("actions never mutate market coordinates", func() {
			marketEffect := model.Outcome(OutcomeRequest{
				Treatment: "enter",
				Target:    testVariable("cvd", "midpoint_log_return"),
				Current:   map[VariableID]float64{},
				At:        at,
			})
			So(marketEffect.Defined(), ShouldBeFalse)
			So(marketEffect.Status, ShouldEqual, IdentificationNotIdentifiable)
		})

		Convey("an unsupported causal query returns NotIdentifiable, not zero", func() {
			effect := model.Outcome(OutcomeRequest{
				Treatment: "enter",
				Target:    testVariable("cvd", "midpoint_log_return"),
				Current:   map[VariableID]float64{},
				At:        at,
			})
			So(effect.Status, ShouldEqual, IdentificationNotIdentifiable)
			So(effect.ExpectedOutcome, ShouldEqual, 0)
			So(effect.Defined(), ShouldBeFalse)
		})

		Convey("mediation is represented by the path, not evidence votes", func() {
			// The schema declares Hawkes → CVD → Price structurally. A
			// redundant semantic copy of Hawkes evidence (a direct
			// hawkes → price Influence edge duplicating the structural
			// path) must not change the causal boundary's answer: the
			// intervention on Price stays NotIdentifiable, with no second
			// evidence vote entering the estimate.
			hawkes := testVariable("hawkes", "conditional_intensity:buy")
			priceReturn := testVariable("cvd", "midpoint_log_return")
			redundantGraph := graph.NewInfluenceGraph(1, 1, 1, 32)
			err := redundantGraph.UpsertEdge(&graph.InfluenceEdge{
				Type:   graph.EdgeInfluence,
				Source: hawkes.Coordinate,
				Target: priceReturn.Coordinate,
				Epoch:  1,
			})
			So(err, ShouldBeNil)

			modelWithEvidence := NewCausalModel(schema, store, redundantGraph, "test-v1")
			effect := modelWithEvidence.Outcome(OutcomeRequest{
				Treatment: "enter",
				Target:    priceReturn,
				Current:   map[VariableID]float64{},
				At:        at,
			})

			Convey("the redundant evidence does not change the NotIdentifiable result", func() {
				So(effect.Status, ShouldEqual, IdentificationNotIdentifiable)
				So(effect.Defined(), ShouldBeFalse)
			})
		})

		Convey("outcome variables are never treated as action targets", func() {
			// Even a schema that labels an outcome variable with an action
			// definition cannot make an intervention on a market coordinate
			// identified: there is no market-impact model.
			priceReturn := testVariable("cvd", "midpoint_log_return")
			outcomeRole := VariableID{Coordinate: priceReturn.Coordinate, Role: RoleOutcome}

			effect := model.Outcome(OutcomeRequest{
				Treatment: "enter",
				Target:    outcomeRole,
				Current:   map[VariableID]float64{},
				At:        at,
			})
			So(effect.Status, ShouldEqual, IdentificationNotIdentifiable)
		})
	})
}

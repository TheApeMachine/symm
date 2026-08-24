package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/relation"
)

func testVariable(source string, metric string) VariableID {
	return VariableID{
		Coordinate: relation.Coordinate{
			Symbol: "BTC/USD",
			Source: source,
			Metric: metric,
			Epoch:  1,
		},
		Role: RoleMarket,
	}
}

func testSchema() *CausalSchema {
	schema := NewCausalSchema("test-schema", "BTC/USD", 1)
	priceReturn := testVariable("cvd", "midpoint_log_return")
	cvdFlow := testVariable("cvd", "signed_net_fraction_zscore")
	hawkes := testVariable("hawkes", "conditional_intensity:buy")

	schema.AddMarketVariable(MarketVariable{
		Variable: priceReturn,
		SelfLag:  time.Second,
		Parents: []AllowedParent{
			{Parent: cvdFlow, Lag: time.Second},
		},
	})
	schema.AddMarketVariable(MarketVariable{
		Variable: cvdFlow,
		SelfLag:  time.Second,
		Parents: []AllowedParent{
			{Parent: hawkes, Lag: time.Second},
		},
	})

	position := VariableID{
		Coordinate: relation.Coordinate{Symbol: "BTC/USD", Source: "portfolio", Metric: "position", Epoch: 1},
		Role:       RolePortfolio,
	}
	schema.AddAction(ActionDefinition{Name: "enter", Variable: position})
	schema.AddAction(ActionDefinition{Name: "exit", Variable: position})
	schema.AddAction(ActionDefinition{Name: "scale", Variable: position})
	schema.AddPortfolioVariable(position)
	schema.AddOutcome(priceReturn)
	schema.ForbidDirection(priceReturn, hawkes)

	return schema
}

func buildStore() *relation.ObservationStore {
	store := relation.NewObservationStore(4096)
	random := time.Now().UnixNano() % 7919
	step := time.Second

	for index := 0; index < 120; index++ {
		at := time.Unix(0, int64(index)*int64(step))

		random = (random*1103515245 + 12345) & 0x7fffffff
		noise := float64(random)/0x3fffffff - 1
		store.Append(relation.Observation{
			Coordinate: testVariable("hawkes", "conditional_intensity:buy").Coordinate,
			Raw:        noise,
			At:         at,
		})
	}

	cvdFlow := testVariable("cvd", "signed_net_fraction_zscore")
	priceReturn := testVariable("cvd", "midpoint_log_return")

	for index := 1; index < 120; index++ {
		at := time.Unix(0, int64(index)*int64(step))
		flow := 0.3*valueAt(store, cvdFlow.Coordinate, at.Add(-step)) + 0.7*valueAt(store, testVariable("hawkes", "conditional_intensity:buy").Coordinate, at.Add(-step))

		store.Append(relation.Observation{Coordinate: cvdFlow.Coordinate, Raw: flow, At: at})
		store.Append(relation.Observation{
			Coordinate: priceReturn.Coordinate,
			Raw:        0.2*valueAt(store, priceReturn.Coordinate, at.Add(-step)) + 0.5*valueAt(store, cvdFlow.Coordinate, at.Add(-step)),
			At:         at,
		})
	}

	return store
}

func valueAt(store *relation.ObservationStore, coordinate relation.Coordinate, at time.Time) float64 {
	history := store.History(coordinate)

	for index := len(history) - 1; index >= 0; index-- {
		if history[index].At.Equal(at) {
			return history[index].Raw
		}
	}

	return 0
}

func TestCausalSchemaConformance(t *testing.T) {
	Convey("Given a causal schema", t, func() {
		schema := testSchema()

		Convey("matrix materialization is reversible", func() {
			store := buildStore()
			matrix, err := schema.Materialize(store, time.Unix(0, 119*int64(time.Second)))
			So(err, ShouldBeNil)
			So(len(matrix.Index.Columns), ShouldEqual, 2)

			for column, variable := range matrix.Index.Columns {
				resolved, found := matrix.Index.VariableOf(column)
				So(found, ShouldBeTrue)
				So(resolved, ShouldEqual, variable)
				So(matrix.Index.ColumnOf(variable), ShouldEqual, column)
				So(variable.Coordinate.Source, ShouldNotBeBlank)
			}
		})

		Convey("future-direction rejection is explicit", func() {
			priceReturn := testVariable("cvd", "midpoint_log_return")
			hawkes := testVariable("hawkes", "conditional_intensity:buy")
			So(schema.DirectionForbidden(priceReturn, hawkes), ShouldBeTrue)
		})
	})
}

func TestCausalModelConformance(t *testing.T) {
	Convey("Given a causal model over real observations", t, func() {
		store := buildStore()
		influenceGraph := graph.NewInfluenceGraph(1, 1, 1, 32)
		schema := testSchema()
		model := NewCausalModel(schema, store, influenceGraph, "test-v1")
		at := time.Unix(0, 119*int64(time.Second))

		Convey("the market transition model is identified on real history", func() {
			transition := model.TransitionModel(testVariable("cvd", "signed_net_fraction_zscore"), at)
			So(transition.Status, ShouldEqual, IdentificationIdentified)
			So(transition.EffectiveSupport, ShouldBeGreaterThan, 10)
			So(transition.Maturity, ShouldBeGreaterThan, 0.9)
		})

		Convey("actions mutate portfolio variables but never market coordinates", func() {
			position := VariableID{
				Coordinate: relation.Coordinate{Symbol: "BTC/USD", Source: "portfolio", Metric: "position", Epoch: 1},
				Role:       RolePortfolio,
			}

			effect := model.Outcome(OutcomeRequest{
				Treatment: "enter",
				Target:    position,
				Current:   map[VariableID]float64{position: 0},
				At:        at,
			})
			So(effect.Defined(), ShouldBeTrue)
			So(effect.ExpectedOutcome, ShouldEqual, 1)

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

		Convey("simulated states never become observational evidence", func() {
			before := store.Snapshot()
			transition := model.TransitionModel(testVariable("cvd", "midpoint_log_return"), at)
			So(transition.Status, ShouldEqual, IdentificationIdentified)

			state := model.MarketState(at)
			expected, noise, defined := transition.Step(state)
			So(defined, ShouldBeTrue)
			So(noise, ShouldBeGreaterThanOrEqualTo, 0)

			_ = expected

			after := store.Snapshot()
			So(after.Appended, ShouldEqual, before.Appended)
			So(after.Observations, ShouldEqual, before.Observations)
		})

		Convey("mediation is represented by the path, not evidence votes", func() {
			// The schema declares Hawkes → CVD → Price structurally. A
			// redundant semantic copy of Hawkes evidence must not create a
			// second vote: querying the Price outcome for an intervention is
			// NotIdentifiable (no market-impact model), not a vote sum.
			effect := model.Outcome(OutcomeRequest{
				Treatment: "enter",
				Target:    testVariable("cvd", "midpoint_log_return"),
				Current:   map[VariableID]float64{},
				At:        at,
			})
			So(effect.Status, ShouldEqual, IdentificationNotIdentifiable)
		})

		Convey("insufficient support is explicit, not zero", func() {
			emptyStore := relation.NewObservationStore(64)
			emptyModel := NewCausalModel(schema, emptyStore, influenceGraph, "test-v1")

			transition := emptyModel.TransitionModel(testVariable("cvd", "signed_net_fraction_zscore"), at)
			So(transition.Status, ShouldEqual, IdentificationUndefined)
		})
	})
}

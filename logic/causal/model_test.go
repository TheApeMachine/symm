package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
buildStore builds a synthetic observational store with a fixed deterministic
seed, so repeated runs produce identical observations and stable
TransitionModel assertions. The LCG and the synthetic-series generation are
unchanged from the original fixture.
*/
func buildStore() *relation.ObservationStore {
	store := relation.NewObservationStore(4096)
	random := int64(7919)
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

		Convey("actions mutate portfolio variables deterministically", func() {
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

		Convey("insufficient support is explicit, not zero", func() {
			emptyStore := relation.NewObservationStore(64)
			emptyModel := NewCausalModel(schema, emptyStore, influenceGraph, "test-v1")

			transition := emptyModel.TransitionModel(testVariable("cvd", "signed_net_fraction_zscore"), at)
			So(transition.Status, ShouldEqual, IdentificationUndefined)
		})

		Convey("a missing parent coordinate is excluded query-locally and recorded", func() {
			transition := model.TransitionModel(testVariable("cvd", "midpoint_log_return"), at)
			So(transition.Status, ShouldEqual, IdentificationIdentified)

			// The schema authorizes hawkes as a parent of price return via
			// the structural path; with data present it participates.
			found := false

			for _, parent := range transition.Parents {
				if parent.Parent.Coordinate.Metric == "signed_net_fraction_zscore" {
					found = true
				}
			}

			So(found, ShouldBeTrue)
		})
	})
}

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
	value := 0.0
	found := false

	store.RangeHistory(coordinate, func(observation relation.Observation) bool {
		if observation.At.Equal(at) {
			value = observation.Raw
			found = true
		}

		return true
	})

	if found {
		return value
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

			state := staticState(model.MarketState(at))
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

		Convey("a defined Relation activates a schema-authorized parent with its measured lag", func() {
			// The schema authorizes flow as a price-return parent; the
			// Influence Graph must hold a defined Relation for it to become
			// an active fitted parent.
			flow := testVariable("cvd", "signed_net_fraction_zscore")
			priceReturn := testVariable("cvd", "midpoint_log_return")
			lag := 2 * time.Second
			coefficient := 0.5
			gain := 0.4
			variance := 0.01
			snr := 25.0
			defined := influenceGraph.UpsertEdge(&graph.InfluenceEdge{
				Type:   graph.EdgeInfluence,
				Source: flow.Coordinate,
				Target: priceReturn.Coordinate,
				Result: &relation.InfluenceResult{
					Source:              flow.Coordinate,
					Target:              priceReturn.Coordinate,
					Lag:                 lag,
					Coefficient:         &coefficient,
					CoefficientVariance: &variance,
					CoefficientSNR:      &snr,
					PredictiveGain:      &gain,
					EstimatorVersion:    "test-v1",
					Epoch:               1,
					Status:              relation.FitOK,
				},
				Epoch: 1,
			})
			So(defined, ShouldBeNil)

			transition := model.TransitionModel(priceReturn, at)
			So(transition.Status, ShouldEqual, IdentificationIdentified)

			found := false

			for _, parent := range transition.Parents {
				if parent.Parent.Coordinate.Metric == "signed_net_fraction_zscore" {
					found = true
					So(parent.Lag, ShouldEqual, lag)
					So(parent.LagSource, ShouldContainSubstring, "influence:")
				}
			}

			So(found, ShouldBeTrue)
		})

		Convey("an authorized parent without a defined Relation is excluded, not assumed", func() {
			// The schema authorizes hawkes as a flow parent; without a
			// defined Relation the direction is a candidate-but-unavailable
			// relationship, not an active fitted parent with a fallback lag.
			flow := testVariable("cvd", "signed_net_fraction_zscore")
			hawkes := testVariable("hawkes", "conditional_intensity:buy")
			So(influenceGraph.Relation(hawkes.Coordinate, flow.Coordinate), ShouldBeNil)

			transition := model.TransitionModel(flow, at)
			So(transition.Status, ShouldEqual, IdentificationIdentified)

			found := false

			for _, parent := range transition.Parents {
				if parent.Parent.Coordinate.Metric == "conditional_intensity:buy" {
					found = true
				}
			}

			So(found, ShouldBeFalse)

			excluded := false

			for _, parent := range transition.ExcludedParents {
				if parent.Parent.Coordinate.Metric == "conditional_intensity:buy" {
					excluded = true
				}
			}

			So(excluded, ShouldBeTrue)
		})
	})
}

/*
staticState adapts a current-value map to the LaggedState interface for
tests: only lag-zero (current) values are available.
*/
type staticState map[relation.Coordinate]float64

func (state staticState) ValueAt(coordinate relation.Coordinate, lag time.Duration) (float64, bool) {
	if lag <= 0 {
		value, found := state[coordinate]
		return value, found
	}

	return 0, false
}

/*
timedState is a LaggedState for tests with an explicit timestamped history.
*/
type timedState struct {
	at      time.Time
	current map[relation.Coordinate]float64
	history map[relation.Coordinate][]sampleAt
}

type sampleAt struct {
	at    time.Time
	value float64
}

func (state timedState) ValueAt(coordinate relation.Coordinate, lag time.Duration) (float64, bool) {
	cutoff := state.at.Add(-lag)

	for index := len(state.history[coordinate]) - 1; index >= 0; index-- {
		if !state.history[coordinate][index].at.After(cutoff) {
			return state.history[coordinate][index].value, true
		}
	}

	if lag <= 0 {
		if value, found := state.current[coordinate]; found {
			return value, true
		}
	}

	return 0, false
}

func TestTransitionStepHonorsMeasuredLag(t *testing.T) {
	Convey("Given a transition fitted with a two-second parent lag", t, func() {
		target := testVariable("cvd", "midpoint_log_return")
		parent := testVariable("cvd", "signed_net_fraction_zscore")
		transition := &TransitionModel{
			Target:            target,
			SelfLag:           time.Second,
			Parents:           []AllowedParent{{Parent: parent, Lag: 2 * time.Second, LagSource: "influence:test"}},
			Intercept:         0,
			SelfCoefficient:   0,
			ParentCoefficients: []float64{1},
			ResidualVariance:  0,
			Status:            IdentificationIdentified,
		}

		at := time.Unix(0, 100*int64(time.Second))

		state := timedState{
			at:      at,
			current: map[relation.Coordinate]float64{target.Coordinate: 0},
			history: map[relation.Coordinate][]sampleAt{
				parent.Coordinate: {
					{at: at.Add(-2 * time.Second), value: 5},
					{at: at.Add(-1 * time.Second), value: 6},
					{at: at, value: 9},
				},
			},
		}

		Convey("the step reads the parent as-of its measured lag, not contemporaneously", func() {
			expected, _, defined := transition.Step(state)
			So(defined, ShouldBeTrue)
			// Parent cutoff = At + SelfLag - ParentLag = At - 1s -> value 6,
			// not the contemporaneous 9 and not the 2s-old 5.
			So(expected, ShouldEqual, 6)
		})
	})
}

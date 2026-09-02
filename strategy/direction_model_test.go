package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestDirectionalStateResolve(t *testing.T) {
	Convey("Given a pending return issued on a three-ticker horizon", t, func() {
		predictor, err := newDirectionalPredictor(directionalConfig{
			initialVariance:  1,
			forgettingFactor: 1,
		})
		So(err, ShouldBeNil)

		state, err := predictor.state("TEST/USD")
		So(err, ShouldBeNil)
		state.horizonSteps = 3
		state.opportunity = types.OpportunityCandidate{
			Symbol: "TEST/USD", Archetype: types.ArchetypeVerticalIgnition,
			Phase: types.PhaseArmed, Direction: types.DirectionLong,
		}
		context := forecastContext{
			archetype:    types.ArchetypeVerticalIgnition,
			phase:        types.PhaseArmed,
			horizonSteps: 3,
		}
		key := featureKey{family: "measurement", source: "test", metric: "context"}
		So(state.observe(
			key, 1, 1, time.Unix(1, 0), featureContext,
		), ShouldBeNil)

		_, err = predictor.advance("TEST/USD", time.Unix(1, 0), 100)
		So(err, ShouldBeNil)
		So(state.pending.issuedOrdinal, ShouldEqual, uint64(1))
		So(state.pending.horizonSteps, ShouldEqual, 3)

		// A later calibrated selection applies only to the next issued target.
		state.horizonSteps = 1
		_, err = predictor.advance("TEST/USD", time.Unix(3_600, 0), 90)
		So(err, ShouldBeNil)
		So(state.returns[3].outcomes, ShouldEqual, uint64(0))
		_, err = predictor.advance("TEST/USD", time.Unix(3_600, 1), 95)
		So(err, ShouldBeNil)
		So(state.returns[3].outcomes, ShouldEqual, uint64(0))

		Convey("wall-clock gaps and later horizon changes cannot resolve it early", func() {
			So(state.pending.horizonSteps, ShouldEqual, 3)
			So(state.pending.present, ShouldBeTrue)
		})

		Convey("event-time provenance cannot relabel the causal step clock", func() {
			ordinal := state.tickerOrdinal
			_, err = predictor.advance(
				"TEST/USD", time.Unix(3_599, 0), 100,
			)
			So(err, ShouldBeNil)
			So(state.tickerOrdinal, ShouldEqual, ordinal+1)
		})

		Convey("the third subsequent ticker resolves it and freezes the new target", func() {
			So(state.observe(
				key, 1, 1, time.Unix(86_400, 0), featureContext,
			), ShouldBeNil)
			_, err = predictor.advance(
				"TEST/USD", time.Unix(86_400, 0), 110,
			)
			So(err, ShouldBeNil)
			So(state.returns[3].outcomes, ShouldEqual, uint64(1))
			So(state.pending.present, ShouldBeTrue)
			So(state.pending.issuedOrdinal, ShouldEqual, uint64(4))
			So(state.pending.horizonSteps, ShouldEqual, 1)
			So(state.features[key].pending, ShouldBeTrue)

			association := state.features[key].associations[context]
			So(association, ShouldNotBeNil)
			So(association.meanOutcome, ShouldAlmostEqual, math.Log(110.0/100.0))

			_, err = predictor.advance(
				"TEST/USD", time.Unix(86_401, 0), 120,
			)
			So(err, ShouldBeNil)
			So(state.returns[3].outcomes, ShouldEqual, uint64(1))
			So(state.returns[1].outcomes, ShouldEqual, uint64(1))
			So(state.features[key].associations, ShouldContainKey, forecastContext{
				archetype:    types.ArchetypeVerticalIgnition,
				phase:        types.PhaseArmed,
				horizonSteps: 1,
			})
		})
	})
}

func TestPredictorFeatureAvailable(t *testing.T) {
	Convey("Given one observed feature", t, func() {
		observedAt := time.Unix(100, 0)
		feature := &predictorFeature{
			quality: 1, observed: true, observedAt: observedAt, observedOrder: 2,
		}

		Convey("it is available once after the causal observation boundary", func() {
			So(feature.available(1), ShouldBeTrue)
			So(feature.available(2), ShouldBeFalse)
		})

		Convey("its provenance timestamp does not change availability", func() {
			feature.observedAt = observedAt.Add(24 * time.Hour)
			So(feature.available(1), ShouldBeTrue)

			feature.observedAt = observedAt.Add(-24 * time.Hour)
			So(feature.available(1), ShouldBeTrue)
		})

		Convey("missing provenance is never silently current", func() {
			feature.observedAt = time.Time{}
			So(feature.available(1), ShouldBeFalse)
		})
	})
}

func TestDirectionalStateObserve(t *testing.T) {
	Convey("Given a previously observed feature coordinate", t, func() {
		state := &directionalState{
			features:     make(map[featureKey]*predictorFeature),
			groupsByName: make(map[string]int),
		}
		key := featureKey{family: "measurement", source: "test", metric: "fact"}
		So(state.observe(
			key, 1, 1, time.Unix(2, 0), featureContext,
		), ShouldBeNil)

		Convey("a later commit replaces the coordinate regardless of event provenance", func() {
			err := state.observe(
				key, 2, 1, time.Unix(1, 0), featureContext,
			)
			So(err, ShouldBeNil)
			So(state.features[key].value, ShouldEqual, 2.0)
			So(state.features[key].observedAt, ShouldResemble, time.Unix(1, 0))
			So(state.features[key].observedOrder, ShouldEqual, uint64(2))
		})
	})
}

package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestSourceTrustStoreWeightColdStart(t *testing.T) {
	convey.Convey("Given an empty trust store", t, func() {
		store := NewSourceTrustStore()

		convey.Convey("It should probation unknown sources below full weight", func() {
			convey.So(store.Weight("depthflow"), convey.ShouldEqual, sourceTrustColdStart)
		})
	})
}

func TestSourceTrustStoreApplyWinningForecast(t *testing.T) {
	convey.Convey("Given a winning forecast", t, func() {
		store := NewSourceTrustStore()
		store.Apply(engine.PredictionFeedback{
			Source:          "hawkes",
			PredictedReturn: 0.01,
			ActualReturn:    0.01,
		})

		convey.Convey("It should keep trust near one", func() {
			convey.So(store.Weight("hawkes"), convey.ShouldBeGreaterThan, 0.8)
		})
	})
}

func TestSourceTrustStoreApplyRepeatedLosses(t *testing.T) {
	convey.Convey("Given repeated losing forecasts", t, func() {
		store := NewSourceTrustStore()

		for range 5 {
			store.Apply(engine.PredictionFeedback{
				Source:          "pumpdump",
				PredictedReturn: 0.01,
				ActualReturn:    -0.01,
			})
		}

		convey.Convey("It should mute the source", func() {
			convey.So(store.Weight("pumpdump"), convey.ShouldEqual, sourceTrustFloor)
		})
	})
}

func TestSourceTrustStorePrefersTightScalpProfile(t *testing.T) {
	convey.Convey("Given one volatile winner and one steady winner", t, func() {
		volatile := NewSourceTrustStore()
		steady := NewSourceTrustStore()

		for range 3 {
			volatile.Apply(engine.PredictionFeedback{
				Source:          "hawkes",
				PredictedReturn: 0.01,
				ActualReturn:    0.015,
			})
			volatile.Apply(engine.PredictionFeedback{
				Source:          "hawkes",
				PredictedReturn: 0.01,
				ActualReturn:    -0.02,
			})
		}

		for range 7 {
			steady.Apply(engine.PredictionFeedback{
				Source:          "fluid",
				PredictedReturn: 0.01,
				ActualReturn:    0.011,
			})
		}

		for range 3 {
			steady.Apply(engine.PredictionFeedback{
				Source:          "fluid",
				PredictedReturn: 0.01,
				ActualReturn:    -0.004,
			})
		}

		convey.Convey("It should rank the steady source above the volatile one", func() {
			convey.So(steady.Weight("fluid"), convey.ShouldBeGreaterThan, volatile.Weight("hawkes"))
		})
	})
}

func TestDecisionEngineBuildUsesTrustWeights(t *testing.T) {
	convey.Convey("Given one trusted and one muted source", t, func() {
		store := NewSourceTrustStore()

		for range 4 {
			store.Apply(engine.PredictionFeedback{
				Source:          "hawkes",
				PredictedReturn: 0.01,
				ActualReturn:    0.01,
			})
		}

		for range 4 {
			store.Apply(engine.PredictionFeedback{
				Source:          "pumpdump",
				PredictedReturn: 0.01,
				ActualReturn:    -0.01,
			})
		}

		candidates := NewCandidateStore()
		candidates.Note(SignalCandidate{
			Symbol:         "PUMP/EUR",
			Source:         "hawkes",
			Confidence:     0.5,
			ExpectedReturn: 0.008,
			Runway:         15,
			Direction:      1,
			Executable:     true,
		})
		candidates.Note(SignalCandidate{
			Symbol:         "PUMP/EUR",
			Source:         "pumpdump",
			Confidence:     0.5,
			ExpectedReturn: 0.008,
			Runway:         15,
			Direction:      1,
			Executable:     true,
		})

		decisionEngine := DecisionEngine{}
		snapshot := decisionEngine.Build(
			candidates,
			stubPrices{"PUMP/EUR": 100},
			stubMarket{snapshots: map[string]engine.Snapshot{
				"PUMP/EUR": {LastOK: true, SpreadOK: true, BatchOK: true},
			}},
			time.Now(),
			200,
			false,
			EnsembleContext{Regime: RegimeChopping, Trust: store},
		)

		convey.Convey("It should weight hawkes more than pumpdump in the combined score", func() {
			convey.So(len(snapshot.Evaluations), convey.ShouldEqual, 1)
			convey.So(snapshot.Evaluations[0].CombinedScore, convey.ShouldAlmostEqual, 0.275, 0.01)
		})
	})
}

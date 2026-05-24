package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

type stubMarket struct {
	snapshots map[string]engine.Snapshot
}

func (market stubMarket) Read(symbol string) engine.Snapshot {
	return market.snapshots[symbol]
}

func TestClassifyMarketRegime(t *testing.T) {
	Convey("Given directional two-sided flow", t, func() {
		regime := ClassifyMarketRegime(stubMarket{snapshots: map[string]engine.Snapshot{
			"A/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.6, BatchOK: true, BatchVolume: 10},
			"B/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.55, BatchOK: true, BatchVolume: 12},
		}}, []string{"A/EUR", "B/EUR"})

		Convey("It should classify trending", func() {
			So(regime, ShouldEqual, RegimeTrending)
		})
	})

	Convey("Given quiet low-activity symbols", t, func() {
		regime := ClassifyMarketRegime(stubMarket{snapshots: map[string]engine.Snapshot{
			"A/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.01},
			"B/EUR": {LastOK: true, PressureOK: true, BuyPressure: -0.01},
		}}, []string{"A/EUR", "B/EUR"})

		Convey("It should classify dead", func() {
			So(regime, ShouldEqual, RegimeDead)
		})
	})
}

func TestRegimeWeight(t *testing.T) {
	Convey("Given a trending regime", t, func() {
		Convey("It should favor hawkes over pumpdump", func() {
			So(RegimeWeight(RegimeTrending, "hawkes"), ShouldBeGreaterThan, RegimeWeight(RegimeTrending, "pumpdump"))
		})
	})
}

func TestSourceTrustStoreApply(t *testing.T) {
	Convey("Given a winning forecast", t, func() {
		store := NewSourceTrustStore()
		store.Apply(engine.PredictionFeedback{
			Source:          "hawkes",
			PredictedReturn: 0.01,
			ActualReturn:    0.01,
		})

		Convey("It should keep trust near one", func() {
			So(store.Weight("hawkes"), ShouldBeGreaterThan, 0.8)
		})
	})

	Convey("Given repeated losing forecasts", t, func() {
		store := NewSourceTrustStore()

		for range 5 {
			store.Apply(engine.PredictionFeedback{
				Source:          "pumpdump",
				PredictedReturn: 0.01,
				ActualReturn:    -0.01,
			})
		}

		Convey("It should mute the source", func() {
			So(store.Weight("pumpdump"), ShouldEqual, sourceTrustFloor)
		})
	})
}

func TestDecisionEngineBuildUsesTrustWeights(t *testing.T) {
	Convey("Given one trusted and one muted source", t, func() {
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
		})
		candidates.Note(SignalCandidate{
			Symbol:         "PUMP/EUR",
			Source:         "pumpdump",
			Confidence:     0.5,
			ExpectedReturn: 0.008,
			Runway:         15,
			Direction:      1,
		})

		engine := DecisionEngine{}
		snapshot := engine.Build(
			candidates,
			stubPrices{"PUMP/EUR": 100},
			false,
			EnsembleContext{Regime: RegimeChopping, Trust: store},
		)

		Convey("It should weight hawkes more than pumpdump in the combined score", func() {
			So(len(snapshot.Evaluations), ShouldEqual, 1)
			So(snapshot.Evaluations[0].CombinedScore, ShouldAlmostEqual, 0.275, 0.01)
		})
	})
}

func TestRequiredEdgeReturnOmitsSpreadDoubleCount(t *testing.T) {
	Convey("Given default fee config and a tight quote", t, func() {
		edge := requiredEdgeReturn(stubPrices{"PUMP/EUR": 100}, "PUMP/EUR")

		Convey("It should not add spread on top of round-trip fees", func() {
			So(edge, ShouldBeLessThan, 0.007)
		})
	})
}

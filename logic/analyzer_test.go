package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
readyStageGate admits every analyzer stage in focused synchronous tests.
*/
type readyStageGate struct{}

/*
Ready reports that the focused analyzer fixture has completed preflight.
*/
func (readyStageGate) Ready(system.StageType) bool {
	return true
}

func TestAnalyzerUpdate(t *testing.T) {
	Convey("Given measurements for two symbols", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Stream: types.Hawkes, Metric: types.MetricArrivalRate,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
			},
			&types.Measurement{
				Stream: types.PumpDump, Metric: types.MetricRVOL,
				Symbol: "ETH/USD", At: time.Unix(2, 0),
			},
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("It should write one symbol-local graph directly onto the thesis", func() {
			So(thesis.Graphs, ShouldHaveLength, 2)
			So(thesis.Graphs["BTC/USD"].Nodes, ShouldHaveLength, 1)
			So(thesis.Graphs["ETH/USD"].Nodes, ShouldHaveLength, 1)
		})
	})
}

func TestAnalyzerUpdateComposesRelationships(t *testing.T) {
	Convey("Given comparable typed measurements on one thesis", t, func() {
		thesis := types.NewThesis(nil)
		positive := 0.5
		negative := -0.5
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Source: types.SourceHawkes, Stream: types.Hawkes,
				Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
				Unit: types.UnitDimensionless, Normalized: &positive,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
			},
			&types.Measurement{
				Source: types.SourcePumpDump, Stream: types.PumpDump,
				Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
				Unit: types.UnitDimensionless, Normalized: &negative,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
			},
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("It should retain the contradiction on the symbol graph", func() {
			So(thesis.Graphs["BTC/USD"].Edges, ShouldHaveLength, 1)
			So(thesis.Graphs["BTC/USD"].Edges[0].Type, ShouldEqual, types.Contradicts)
		})
	})
}

func TestAnalyzerIngestLevel3(t *testing.T) {
	Convey("Given authoritative L3 rows on the current Thesis", t, func() {
		viper.Set("market.l3_depth", 10)
		viper.Set("market.forecast.rls.initial_variance", 1.0)
		viper.Set("market.forecast.rls.forgetting_factor", 1.0)
		viper.Set("market.manifold.lifetime_capacity", 256)
		analyzer := NewAnalyzer(readyStageGate{}, nil)
		defer analyzer.Close()
		thesis := types.NewThesis(nil)
		thesis.Signals.Store("level3", []kraken.Level3Data{
			{
				Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
				Bids: []kraken.Level3Order{{
					OrderID: "bid-1", LimitPrice: 99, OrderQty: 2,
					Timestamp: time.Unix(1, 0),
				}},
				Asks: []kraken.Level3Order{{
					OrderID: "ask-1", LimitPrice: 101, OrderQty: 3,
					Timestamp: time.Unix(1, 0),
				}},
			},
		})

		analyzer.Update(thesis)

		Convey("Then the symbol is admitted and its tick epoch is consumed", func() {
			slot, exists := analyzer.engine.Slot("BTC/USD")
			So(exists, ShouldBeTrue)
			So(slot.Advance().StateProduced, ShouldBeFalse)
		})
	})
}

func TestAnalyzerHandle(t *testing.T) {
	Convey("Given a manifold result carrying a durable forecast", t, func() {
		thesis := types.NewThesis(nil)
		analyzer := &Analyzer{}
		forecast := types.Forecasts{
			Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		}

		analyzer.handle("BTC/USD", thesis, manifold.ProcessResult{
			Forecast: &forecast,
		})

		Convey("It should append the forecast directly to the thesis", func() {
			So(thesis.Forecasts, ShouldResemble, []types.Forecasts{forecast})
		})
	})
}

func TestAnalyzerHandleRetainsLogicEvidence(t *testing.T) {
	Convey("Given chronological manifold states reaching the analyzer", t, func() {
		thesis := types.NewThesis(nil)
		analyzer := &Analyzer{
			resonances: map[string]*Resonance{
				"BTC/USD": NewResonance(
					"BTC/USD", nil, manifold.DefaultBaselineHalflife,
				),
			},
			causals: map[string]*Causal{
				"BTC/USD": NewCausal("BTC/USD", nil),
			},
			replay: manifold.NewReplayRecorder(),
		}

		for index := 1; index <= 16; index++ {
			state := causalState(
				time.Unix(int64(index), 0),
				100+float64(index),
				uint64(index),
			)
			state.PressureGradX += float64(index) / 100
			analyzer.handle("BTC/USD", thesis, manifold.ProcessResult{
				State: state, GasReady: true,
			})
		}

		Convey("It should retain resonance measurements and causal hypotheses", func() {
			So(thesis.Measurements, ShouldNotBeEmpty)
			So(thesis.Hypotheses, ShouldHaveLength, 16)
			So(thesis.Hypotheses[0].Claim, ShouldEqual, causalHypothesis)
		})
	})
}

func BenchmarkAnalyzerUpdate(b *testing.B) {
	analyzer := &Analyzer{}

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements, &types.Measurement{
			Stream: types.Hawkes, Metric: types.MetricArrivalRate,
			Symbol: "BTC/USD", At: time.Unix(1, 0),
		})
		analyzer.Update(thesis)
	}
}

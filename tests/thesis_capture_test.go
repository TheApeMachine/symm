package tests

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

const benchmarkCycleObservations = 128

func TestCloneThesisEvidence(t *testing.T) {
	Convey("Given analyzed evidence at the planner boundary", t, func() {
		thesis := types.NewThesis(nil)
		observedAt := time.Unix(1_700_007_000, 0).UTC()
		measurement := &types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "SIM1/USD",
			At:     observedAt,
		}
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "SIM1/USD", Timestamp: observedAt,
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "SIM1/USD", TradeID: 1, Timestamp: observedAt,
		})
		thesis.AppendMeasurements(types.SourcePumpDump, measurement)
		thesis.Categories["SIM1/USD"] = []types.Category{{
			Symbol: "SIM1/USD", Type: types.VerticalIgnition,
		}}
		forecast, err := types.NewResonanceForecast(
			[]float64{0.01}, []float64{1}, 1, 0.5,
		)
		So(err, ShouldBeNil)
		thesis.Resonance.Store("SIM1/USD", types.ResonanceReading{
			Symbol: "SIM1/USD", Forecast: forecast,
			ForecastValidity: types.MeasurementValidity{
				State: types.ValidityValid, Readiness: types.ReadinessForecast,
			},
		})

		Convey("The capture should survive closure of the live cycle", func() {
			snapshot := cloneThesisEvidence(thesis)
			thesis.CloseCycle()

			So(snapshot.MarketTickers(), ShouldHaveLength, 1)
			So(snapshot.MarketTrades(), ShouldHaveLength, 1)
			So(snapshot.Series("SIM1/USD"), ShouldHaveLength, 1)
			So(snapshot.Categories["SIM1/USD"], ShouldHaveLength, 1)
			_, found := snapshot.Resonance.Load("SIM1/USD")
			So(found, ShouldBeTrue)
		})
	})
}

func BenchmarkCloneThesisEvidence(b *testing.B) {
	thesis := types.NewThesis(nil)
	observedAt := time.Unix(1_700_007_000, 0).UTC()

	for sequence := range benchmarkCycleObservations {
		at := observedAt.Add(time.Duration(sequence) * time.Millisecond)
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "SIM1/USD", Timestamp: at,
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "SIM1/USD", TradeID: int64(sequence + 1), Timestamp: at,
		})
	}

	for b.Loop() {
		_ = cloneThesisEvidence(thesis)
	}
}

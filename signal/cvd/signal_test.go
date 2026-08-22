package cvd

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func cvdTrade(
	tradeID int64,
	side string,
	price float64,
	qty float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    "BTC/USD",
		TradeID:   tradeID,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func TestCVDNumber(t *testing.T) {
	Convey("Given a sequence of buy then sell executions on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		start := time.Unix(1_700_003_200, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendTrade(cvdTrade(1, "buy", 100.0, 2, start))
		market.AppendTrade(cvdTrade(2, "sell", 100.0, 1, start.Add(time.Second)))

		Convey("It should emit flow decomposition with exact net flow", func() {
			measurements := drainCVDMeasurements(market, 2)

			So(len(measurements), ShouldEqual, 2)

			// First trade is buy of 2 @ 100 -> +200 notional
			So(measurements[0].Metrics["net"].Raw, ShouldEqual, 200)

			// Second trade is sell of 1 @ 100 -> -100 notional
			So(measurements[1].Metrics["net"].Raw, ShouldEqual, -100)

			for _, measurement := range measurements {
				_, hasBaseline := measurement.Metrics["flow_baseline"]
				So(hasBaseline, ShouldBeTrue)

				_, hasZScore := measurement.Metrics["flow_zscore"]
				So(hasZScore, ShouldBeTrue)

				absorption, hasAbsorption := measurement.Metrics["absorption"]
				So(hasAbsorption, ShouldBeTrue)
				So(absorption.Raw, ShouldBeGreaterThanOrEqualTo, 0)
			}
		})
	})

	Convey("Adversarial: Zero quantity and extreme trade bursts", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		start := time.Unix(1_700_003_200, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendTrade(cvdTrade(3, "buy", 100.0, 0, start))
		market.AppendTrade(cvdTrade(4, "buy", 100.0, 10000, start.Add(time.Second)))

		measurements := drainCVDMeasurements(market, 2)

		So(len(measurements), ShouldEqual, 2)
		So(measurements[0].Metrics["net"].Raw, ShouldEqual, 0)
		So(measurements[1].Metrics["net"].Raw, ShouldEqual, 1000000)
	})
}

func drainCVDMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
	readings := []*nmtypes.Measurement{}
	deadline := time.Now().Add(2 * time.Second)

	for len(readings) < expected && time.Now().Before(deadline) {
		for measurement := range symbol.MarketMeasurements(
			symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
		) {
			readings = append(readings, measurement)
		}

		if len(readings) >= expected {
			break
		}

		time.Sleep(time.Millisecond)
	}

	return readings
}

func BenchmarkCVDPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("BTC/USD")
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	start := time.Unix(1_700_003_200, 0).UTC()
	trade := cvdTrade(1, "buy", 100.0, 2, start)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.AppendTrade(trade)
	}
}

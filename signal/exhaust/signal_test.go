package exhaust

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

func exhaustTrade(
	tradeID int64,
	side string,
	price float64,
	qty float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    "AAA/USD",
		TradeID:   tradeID,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func TestExhaustNumber(t *testing.T) {
	Convey("Given trades on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		start := time.Unix(1_700_001_000, 0).UTC()
		market.AppendTrade(exhaustTrade(1, "buy", 100.0, 10.0, start))

		Convey("It should emit an exhaustion reading through the pipeline", func() {
			measurements := drainExhaustMeasurements(market, 1)

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, string(types.SourceExhaustion))

			_, hasMechanical := measurements[0].Metrics["mechanical"]
			So(hasMechanical, ShouldBeTrue)

			_, hasUrgency := measurements[0].Metrics["urgency"]
			So(hasUrgency, ShouldBeTrue)
		})

		Convey("Adversarial: Multi-trade sequence with price rejection and reversal", func() {
			// 1. Buy trade at 100
			market.AppendTrade(exhaustTrade(2, "buy", 100.0, 50.0, start.Add(time.Second)))
			// 2. Buy trade meeting price rejection (price drops to 99 despite buying)
			market.AppendTrade(exhaustTrade(3, "buy", 99.0, 50.0, start.Add(2*time.Second)))
			// 3. Heavy sell reversal
			market.AppendTrade(exhaustTrade(4, "sell", 98.0, 100.0, start.Add(3*time.Second)))

			measurements := drainExhaustMeasurements(market, 3)

			So(len(measurements), ShouldBeGreaterThanOrEqualTo, 2)

			for _, measurement := range measurements {
				urgency, hasUrgency := measurement.Metrics["urgency"]
				So(hasUrgency, ShouldBeTrue)
				So(urgency.Raw, ShouldBeGreaterThanOrEqualTo, 0)
			}
		})
	})
}

func drainExhaustMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
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

func BenchmarkExhaustPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("AAA/USD")
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	start := time.Unix(1_700_001_000, 0).UTC()
	trade := exhaustTrade(1, "buy", 100.0, 10.0, start)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.AppendTrade(trade)
	}
}

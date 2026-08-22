package liquidity

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

func tickerRow(
	symbol string,
	price, quantity, volume float64,
	at time.Time,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(price - 0.5),
		BidQty:    quantity,
		Ask:       decimal.NewFromFloat64(price + 0.5),
		AskQty:    quantity,
		Last:      decimal.NewFromFloat64(price),
		Volume:    volume,
		Vwap:      price,
		Timestamp: at,
	}
}

func TestLiquidityPipeline(t *testing.T) {
	Convey("Given a thesis with a symbol carrying a stable book", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		for index := range 8 {
			at := start.Add(time.Duration(index) * time.Second)
			market.AppendTicker(tickerRow("AAA/USD", 100, 2, 10, at))
		}

		Convey("It should emit one measurement per tick through its own pipeline", func() {
			readings := drainMeasurements(market, 8)

			So(len(readings), ShouldEqual, 8)

			last := readings[len(readings)-1]

			So(last.Source, ShouldEqual, string(types.SourceLiquidity))

			value, found := last.Metrics["executable_touch_depth"]

			So(found, ShouldBeTrue)
			// Depth = min(2, 2) * (99.5 + 100.5)/2 = 2 * 100 = 200
			So(value.Raw, ShouldEqual, 200)

			baseline, found := last.Metrics["depth_baseline"]
			So(found, ShouldBeTrue)
			So(baseline.Raw, ShouldEqual, 200)

			zscore, found := last.Metrics["depth_zscore"]
			So(found, ShouldBeTrue)
			So(zscore.Raw, ShouldEqual, 0)
		})
	})

	Convey("Adversarial: Dynamic response to sudden liquidity collapse and surge", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		// 1. Establish baseline at qty=10 (depth=1000)
		for index := range 5 {
			at := start.Add(time.Duration(index) * time.Second)
			market.AppendTicker(tickerRow("AAA/USD", 100, 10, 100, at))
		}
		drainMeasurements(market, 5)

		// 2. Sudden liquidity dry-up to qty=0.1 (depth=10)
		market.AppendTicker(tickerRow("AAA/USD", 100, 0.1, 100, start.Add(6*time.Second)))
		dryReadings := drainMeasurements(market, 1)

		So(len(dryReadings), ShouldEqual, 1)
		dryDepth := dryReadings[0].Metrics["executable_touch_depth"].Raw
		So(dryDepth, ShouldEqual, 10)

		// Z-score must be negative during collapse
		dryZ := dryReadings[0].Metrics["depth_zscore"].Raw
		So(dryZ, ShouldBeLessThan, 0)

		// 3. Sudden liquidity surge to qty=100 (depth=10000)
		market.AppendTicker(tickerRow("AAA/USD", 100, 100, 100, start.Add(7*time.Second)))
		surgeReadings := drainMeasurements(market, 1)

		So(len(surgeReadings), ShouldEqual, 1)
		surgeDepth := surgeReadings[0].Metrics["executable_touch_depth"].Raw
		So(surgeDepth, ShouldEqual, 10000)

		// Z-score must be positive during surge
		surgeZ := surgeReadings[0].Metrics["depth_zscore"].Raw
		So(surgeZ, ShouldBeGreaterThan, 0)
	})
}

func drainMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
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

func BenchmarkLiquidityPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("AAA/USD")
	start := time.Unix(1_700_000_100, 0).UTC()
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	ticker := tickerRow("AAA/USD", 100, 2, 10, start)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.AppendTicker(ticker)
	}
}

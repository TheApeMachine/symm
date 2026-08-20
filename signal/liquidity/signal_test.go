package liquidity

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken"
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

		for index := range 8 {
			at := start.Add(time.Duration(index) * time.Second)
			market.AppendTicker(tickerRow("AAA/USD", 100, 2, 10, at))
		}

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit one measurement per tick through its own pipeline", func() {
			readings := drainMeasurements(market, 8)

			So(len(readings), ShouldEqual, 8)

			last := readings[len(readings)-1]

			So(last.Source, ShouldEqual, string(types.SourceLiquidity))

			value, found := last.Metrics["executable_touch_depth"]

			So(found, ShouldBeTrue)
			So(value.Raw, ShouldBeGreaterThan, 0)
		})
	})
}

func drainMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
	readings := []*nmtypes.Measurement{}

	deadline := time.Now().Add(2 * time.Second)

	for len(readings) < expected && time.Now().Before(deadline) {
		for measurement := range symbol.MarketMeasurements("category") {
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

	for index := range b.N {
		at := start.Add(time.Duration(index) * time.Second)
		market.AppendTicker(tickerRow("AAA/USD", 100, 2, 10, at))
	}

	signal := NewSignal(context.Background(), thesis)

	b.Run("run", func(b *testing.B) {
		for range b.N {
			_ = signal
		}
	})
}

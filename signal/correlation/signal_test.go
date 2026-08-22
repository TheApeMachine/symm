package correlation

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

func correlationTicker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(price - 0.1),
		BidQty:    10,
		Ask:       decimal.NewFromFloat64(price + 0.1),
		AskQty:    10,
		Last:      decimal.NewFromFloat64(price),
		Volume:    100,
		Vwap:      price,
		Timestamp: at,
	}
}

func TestCorrelationSignal(t *testing.T) {
	Convey("Given multiple market streams for correlation cross-section", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		marketA := thesis.Symbol("AAA/USD")
		marketB := thesis.Symbol("BBB/USD")
		start := time.Unix(1_700_000_000, 0).UTC()

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		// Stream correlated ticks across AAA and BBB
		for index := range 5 {
			at := start.Add(time.Duration(index) * time.Second)
			marketA.AppendTicker(correlationTicker("AAA/USD", 100.0+float64(index)*2.0, at))
			marketB.AppendTicker(correlationTicker("BBB/USD", 50.0+float64(index)*1.0, at))
		}

		measurements := drainCorrelationMeasurements(marketA, 3)

		Convey("It should emit correlation measurements across the cohort", func() {
			So(len(measurements), ShouldBeGreaterThanOrEqualTo, 1)
			last := measurements[len(measurements)-1]
			So(last.Source, ShouldEqual, string(types.SourceCorrelation))
		})
	})
}

func drainCorrelationMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
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

func BenchmarkCorrelationPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("AAA/USD")
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	start := time.Unix(1_700_000_000, 0).UTC()
	ticker := correlationTicker("AAA/USD", 100.0, start)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.AppendTicker(ticker)
	}
}

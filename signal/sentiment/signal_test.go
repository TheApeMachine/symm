package sentiment

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func sentimentTicker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(price),
		Timestamp: at,
	}
}

func drainMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
	readings := make([]*nmtypes.Measurement, 0, expected)
	deadline := time.Now().Add(3 * time.Second)

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

func TestSentimentNumber(t *testing.T) {
	Convey("Given ticker observations on the live callback path", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendTicker(sentimentTicker("AAA/USD", 100.0, start))
		market.AppendTicker(sentimentTicker("AAA/USD", 102.0, start.Add(time.Second)))

		Convey("It should emit measurements through the nomagique stream", func() {
			measurements := drainMeasurements(market, 2)

			So(signal.Error(), ShouldBeNil)
			So(len(measurements), ShouldEqual, 2)
			So(measurements[0].Source, ShouldEqual, string(types.SourceSentiment))
			So(measurements[1].Source, ShouldEqual, string(types.SourceSentiment))
			So(measurements[0].Metric("breadth").Name, ShouldEqual, "breadth")
			So(measurements[1].Metric("breadth").Name, ShouldEqual, "breadth")
		})
	})

	Convey("Given concurrent symbols with streaming event clocks", t, func() {
		const (
			symbolCount     = 32
			eventsPerSymbol = 4
		)

		thesis := types.NewThesis(context.Background(), nil)
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		symbols := make([]*types.Symbol, symbolCount)
		var writers sync.WaitGroup

		for symbolIndex := range symbolCount {
			symbols[symbolIndex] = thesis.Symbol(
				strconv.Itoa(symbolIndex) + "/USD",
			)
			writers.Add(1)

			go func(symbolIndex int) {
				defer writers.Done()

				for eventIndex := range eventsPerSymbol {
					offset := symbolIndex*eventsPerSymbol + eventIndex
					symbols[symbolIndex].AppendTicker(sentimentTicker(
						symbols[symbolIndex].Symbol,
						100.0+float64(offset),
						start.Add(time.Duration(offset)*time.Millisecond),
					))
				}
			}(symbolIndex)
		}

		writers.Wait()

		Convey("It should process symbols concurrently without errors", func() {
			total := 0

			for _, symbol := range symbols {
				readings := drainMeasurements(symbol, eventsPerSymbol)
				total += len(readings)
			}

			So(signal.Error(), ShouldBeNil)
			So(total, ShouldEqual, symbolCount*eventsPerSymbol)
		})
	})

	Convey("Given a multi-symbol cohort with outsized moves and opposite-direction movers", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		arb := thesis.Symbol("ARB/USD")
		btc := thesis.Symbol("BTC/USD")
		eth := thesis.Symbol("ETH/USD")

		arb.AppendTicker(sentimentTicker("ARB/USD", 1.0, start))
		btc.AppendTicker(sentimentTicker("BTC/USD", 100.0, start))
		eth.AppendTicker(sentimentTicker("ETH/USD", 10.0, start))
		_ = drainMeasurements(arb, 1)
		_ = drainMeasurements(btc, 1)
		_ = drainMeasurements(eth, 1)

		next := start.Add(time.Second)
		arb.AppendTicker(sentimentTicker("ARB/USD", 1.2, next))
		btc.AppendTicker(sentimentTicker("BTC/USD", 101.0, next))
		eth.AppendTicker(sentimentTicker("ETH/USD", 9.9, next))

		Convey("Emitted measurements should have valid bounds on all normalized metrics", func() {
			measurements := drainMeasurements(arb, 1)

			So(len(measurements), ShouldEqual, 1)
			latest := measurements[0]

			changeMetric := latest.Metric(string(types.MetricChange))
			So(changeMetric, ShouldNotBeNil)
			So(changeMetric.Normalized, ShouldBeNil)

			leaderMetric := latest.Metric(string(types.MetricLeaderStrength))
			So(leaderMetric, ShouldNotBeNil)
			So(leaderMetric.Normalized, ShouldBeNil)

			surgeMetric := latest.Metric(string(types.MetricSurgeScore))
			So(surgeMetric, ShouldNotBeNil)
			So(surgeMetric.Normalized, ShouldNotBeNil)
			So(*surgeMetric.Normalized, ShouldBeBetweenOrEqual, 0, 1)

			divergentMetric := latest.Metric(string(types.MetricDivergentScore))
			So(divergentMetric, ShouldNotBeNil)
			So(divergentMetric.Normalized, ShouldNotBeNil)
			So(*divergentMetric.Normalized, ShouldBeBetweenOrEqual, 0, 1)

			strengthMetric := latest.Metric(string(types.MetricStrength))
			So(strengthMetric, ShouldNotBeNil)
			So(strengthMetric.Normalized, ShouldNotBeNil)
			So(*strengthMetric.Normalized, ShouldBeBetweenOrEqual, 0, 1)
		})
	})
}

func BenchmarkSentimentNumber(b *testing.B) {
	const cohortSize = 200

	start := time.Unix(1_700_000_000, 0).UTC()
	b.ReportAllocs()

	for b.Loop() {
		thesis := types.NewThesis(context.Background(), nil)
		signal := NewSignal(context.Background(), thesis)

		for symbolIndex := range cohortSize {
			symbol := thesis.Symbol("AAA/USD")
			symbol.AppendTicker(sentimentTicker(
				symbol.Symbol,
				100.0+float64(symbolIndex),
				start.Add(time.Duration(symbolIndex)*time.Nanosecond),
			))
		}

		signal.Close()
	}
}

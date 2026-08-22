package leadlag

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

var benchmarkTickerPrice float64

func leadLagTicker(symbol string, price float64, at time.Time) kraken.TickerData {
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

func TestTickerPrice(t *testing.T) {
	Convey("Given Kraken ticker price states", t, func() {
		Convey("A zero last represents no observation", func() {
			last := decimal.NewFromInt64(0)
			price, observed, err := tickerPrice(kraken.TickerData{Last: last})

			So(err, ShouldBeNil)
			So(observed, ShouldBeFalse)
			So(price, ShouldEqual, 0.0)
		})

		Convey("A positive last is admitted unchanged", func() {
			last := decimal.NewFromInt64(310)
			price, observed, err := tickerPrice(kraken.TickerData{Last: last})

			So(err, ShouldBeNil)
			So(observed, ShouldBeTrue)
			So(price, ShouldEqual, 310.0)
		})

		Convey("A missing last is an explicit error", func() {
			_, _, err := tickerPrice(kraken.TickerData{})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"leadlag: ticker requires a last price")
		})
	})
}

func TestLeadLagSignalPipeline(t *testing.T) {
	Convey("Given multiple symbols for lead-lag evaluation", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		marketLeader := thesis.Symbol("LEADER/USD")
		marketFollower := thesis.Symbol("FOLLOWER/USD")
		start := time.Unix(1_700_000_000, 0).UTC()

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		// Stream leader leading follower by 1 second
		for index := range 6 {
			at := start.Add(time.Duration(index) * time.Second)
			marketLeader.AppendTicker(leadLagTicker("LEADER/USD", 100.0+float64(index)*5.0, at))
			marketFollower.AppendTicker(leadLagTicker("FOLLOWER/USD", 50.0+float64(index)*2.0, at))
		}

		measurements := drainLeadLagMeasurements(marketFollower, 2)

		Convey("It should emit lead-lag measurements for follower", func() {
			So(len(measurements), ShouldBeGreaterThanOrEqualTo, 1)
			last := measurements[len(measurements)-1]
			So(last.Source, ShouldEqual, string(types.SourceLeadLag))
		})
	})
}

func drainLeadLagMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
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

func BenchmarkTickerPrice(b *testing.B) {
	last := decimal.NewFromInt64(310)
	ticker := kraken.TickerData{Last: last}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		benchmarkTickerPrice, _, _ = tickerPrice(ticker)
	}
}

func BenchmarkLeadLagPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("FOLLOWER/USD")
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	start := time.Unix(1_700_000_000, 0).UTC()
	ticker := leadLagTicker("FOLLOWER/USD", 50.0, start)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.AppendTicker(ticker)
	}
}

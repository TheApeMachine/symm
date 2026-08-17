package leadlag

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given repeated multi-symbol lead-lag cuts", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("AAA/USD", nil)
		start := time.Unix(1_700_007_000, 0).UTC()
		var measurements []*types.Measurement

		Reset(func() {
			signal.Close()
		})

		for leg, prices := range [][]float64{
			{100, 100, 100},
			{110, 101, 99},
			{121, 102, 98},
			{133, 103, 97},
		} {
			at := start.Add(time.Duration(leg) * time.Second)

			for index, symbol := range []string{"AAA/USD", "BBB/USD", "CCC/USD"} {
				market.AppendTicker(kraken.TickerData{
					Symbol:    symbol,
					Last:      decimal.NewFromFloat64(prices[index]),
					Timestamp: at,
				}, types.TickerReceivers)
			}

			measurements = slices.Collect(signal.Measure(market))
		}

		Convey("It should retain symbol history and emit nomagique lag measurements", func() {
			So(signal.section.PriceSampleCount("AAA/USD"), ShouldEqual, 4)
			So(signal.section.PriceSampleCount("BBB/USD"), ShouldEqual, 4)
			So(signal.section.PriceSampleCount("CCC/USD"), ShouldEqual, 4)
			So(len(measurements), ShouldBeGreaterThan, 0)
			So(measurements[0].Source, ShouldEqual, types.SourceLeadLag)
			So(measurements[0].Sample(
				types.MetricSampleCount,
				types.SideNone,
			).Normalized, ShouldBeNil)
			So(measurements[0].Sample(
				types.MetricSampleCount,
				types.SideNone,
			).Raw, ShouldBeGreaterThan, 0)
			So(measurements[0].Sample(
				types.MetricSampleCount,
				types.SideNone,
			).Unit, ShouldEqual, types.UnitCount)
			So(measurements[0].Sample(types.MetricStrength, types.SideNone).Unit,
				ShouldEqual, types.UnitDimensionless)
		})
	})

	Convey("Given a retained anchor without a ticker in the current frame", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BBB/USD", nil)
		start := time.Unix(1_700_008_000, 0).UTC()

		for _, sample := range []struct {
			symbol string
			first  float64
			second float64
		}{
			{symbol: "AAA/USD", first: 100, second: 120},
			{symbol: "BBB/USD", first: 100, second: 105},
			{symbol: "CCC/USD", first: 100, second: 101},
		} {
			So(signal.section.ObservePrice(sample.symbol, sample.first, start),
				ShouldBeTrue)
			So(signal.section.ObservePrice(
				sample.symbol, sample.second, start.Add(time.Second),
			), ShouldBeTrue)
		}

		market.AppendTicker(kraken.TickerData{
			Symbol: "BBB/USD", Last: decimal.NewFromFloat64(106),
			Timestamp: start.Add(2 * time.Second),
		}, types.TickerReceivers)
		measurements := slices.Collect(signal.Measure(market))

		Convey("It should retain the exact anchor endpoint on the relationship", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Symbol, ShouldEqual, "BBB/USD")
			So(measurement.Peer, ShouldEqual, "AAA/USD")
			So(measurement.PeerAt, ShouldEqual, start.Add(time.Second))
			So(measurement.PeerObservedFrom, ShouldEqual, start)
			So(measurement.Sample(types.MetricPeerLastPrice, types.SideNone).Raw,
				ShouldEqual, 120.0)
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	signal := NewSignal(context.Background(), nil)
	market := types.NewSymbol("AAA/USD", nil)
	at := time.Unix(1_700_009_000, 0).UTC()

	if !signal.section.ObservePrice("AAA/USD", 100, at) ||
		!signal.section.ObservePrice("AAA/USD", 120, at.Add(time.Second)) {
		b.Fatal("failed to seed anchor prices")
	}

	tickers := make([]kraken.TickerData, 64)

	for index := range tickers {
		symbol := "SIM" + strconv.Itoa(index) + "/USD"

		if !signal.section.ObservePrice(symbol, 100, at) ||
			!signal.section.ObservePrice(symbol, 101, at.Add(time.Second)) {
			b.Fatal("failed to seed follower prices")
		}

		tickers[index] = kraken.TickerData{
			Symbol: symbol,
			Last:   decimal.NewFromFloat64(102),
		}
	}

	at = at.Add(time.Second)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		at = at.Add(time.Second)

		for index := range tickers {
			tickers[index].Timestamp = at
			market.AppendTicker(tickers[index], types.TickerReceivers)
		}

		_ = slices.Collect(signal.Measure(market))
	}
}

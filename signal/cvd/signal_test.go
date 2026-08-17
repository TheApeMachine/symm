package cvd

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given CVD trade observations on one symbol", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Unix(1_700_003_200, 0).UTC()
		market.AppendTicker(cvdTicker(99.99, 100.01, at), types.TickerReceivers)
		trade := cvdTrade(1, "buy", 100.01, at.Add(time.Second))
		market.AppendTrade(trade, types.TradeReceivers)

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should emit flow metrics from nomagique", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourceCVD)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.At, ShouldResemble, trade.Timestamp)
			So(measurement.Sample(types.MetricTradePrice, types.SideNone).Raw, ShouldEqual, 100.01)
			So(measurement.Sample(types.MetricTradeQuantity, types.SideNone).Raw, ShouldEqual, 2.0)
			So(measurement.Sample(types.MetricMidpoint, types.SideNone).Raw, ShouldEqual, 100.0)
			So(measurement.Sample(types.MetricNet, types.SideNone).Unit, ShouldEqual, types.UnitQuoteCurrency)
			So(measurement.Sample(types.MetricNetFraction, types.SideNone).Normalized, ShouldNotBeNil)
		})
	})

	Convey("Given alternating executions around a constant midpoint", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Unix(1_700_003_300, 0).UTC()
		market.AppendTicker(cvdTicker(99.99, 100.01, at), types.TickerReceivers)

		for index := 0; index < 8; index++ {
			side := "buy"
			price := 100.01

			if index%2 == 1 {
				side = "sell"
				price = 99.99
			}

			market.AppendTrade(cvdTrade(
				int64(index+1),
				side,
				price,
				at.Add(time.Duration(index+1)*time.Second),
			), types.TradeReceivers)
		}

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should not turn execution bounce into directional response", func() {
			So(measurements, ShouldHaveLength, 8)

			for _, measurement := range measurements {
				So(measurement.Sample(types.MetricMidpoint, types.SideNone).Raw, ShouldEqual, 100.0)
				So(measurement.Sample(types.MetricDrive, types.SideNone).Raw, ShouldEqual, 0.0)
			}
		})
	})
}

func cvdTicker(bid, ask float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    "BTC/USD",
		Bid:       decimal.NewFromFloat64(bid),
		Ask:       decimal.NewFromFloat64(ask),
		Timestamp: at,
	}
}

func cvdTrade(id int64, side string, price float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: 2, TradeID: id, Timestamp: at,
	}
}

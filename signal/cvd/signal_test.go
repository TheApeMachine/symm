package cvd

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given CVD trade observations on one symbol", t, func() {
		signal := NewSignal(context.Background(), nil, nil)
		market := types.NewSymbol("BTC/USD", nil)
		trade := cvdTrade(1, "buy", 100, time.Unix(1_700_003_200, 0).UTC())
		market.AppendTrade(trade)

		measurements := signal.Measure(market)

		Convey("It should emit flow metrics from nomagique", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourceCVD)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.At, ShouldResemble, trade.Timestamp)
			So(measurement.Sample(types.MetricTradePrice, types.SideNone).Raw, ShouldEqual, 100.0)
			So(measurement.Sample(types.MetricTradeQuantity, types.SideNone).Raw, ShouldEqual, 2.0)
			So(measurement.Sample(types.MetricNet, types.SideNone).Unit, ShouldEqual, types.UnitQuoteCurrency)
			So(measurement.Sample(types.MetricNetFraction, types.SideNone).Normalized, ShouldNotBeNil)
		})
	})
}

func cvdTrade(id int64, side string, price float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: 2, TradeID: id, Timestamp: at,
	}
}

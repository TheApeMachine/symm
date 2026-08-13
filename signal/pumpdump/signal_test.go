package pumpdump

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given pumpdump trade observations on one symbol", t, func() {
		signal := &Signal{
			ctx:    context.Background(),
			algo:   equation.NewIgnition(128),
			quotes: types.NewQuoteHistory(128),
		}
		market := types.NewSymbol("BTC/USD", nil)
		market.AppendTrade(pumpdumpTrade(1, "buy", 100, time.Unix(1_700_002_200, 0).UTC()))

		measurements := signal.Measure(market)

		Convey("It should keep the signal contract while nomagique rejects incomplete quote evidence", func() {
			So(measurements, ShouldBeEmpty)
			So(signal.Name(), ShouldEqual, string(types.SourcePumpDump))
			So(signal.Type(), ShouldEqual, types.SourcePumpDump)
		})
	})

	Convey("Given pumpdump trade observations after a quoted market", t, func() {
		signal := &Signal{
			ctx:    context.Background(),
			algo:   equation.NewIgnition(128),
			quotes: types.NewQuoteHistory(128),
		}
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Unix(1_700_002_300, 0).UTC()
		market.AppendTicker(kraken.TickerData{
			Symbol: "BTC/USD", Bid: decimal.NewFromFloat64(99),
			Ask: decimal.NewFromFloat64(101), Timestamp: at,
		})
		market.AppendTrade(pumpdumpTrade(1, "buy", 100, at.Add(time.Second)))

		measurements := signal.Measure(market)

		Convey("It should emit ignition evidence from the retained quote", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourcePumpDump)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.Sample(types.MetricBestPrice, types.SideBuy).Raw, ShouldEqual, 99.0)
			So(measurement.Sample(types.MetricBestPrice, types.SideSell).Raw, ShouldEqual, 101.0)
		})
	})
}

func pumpdumpTrade(
	id int64,
	side string,
	price float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: 20, TradeID: id, Timestamp: at,
	}
}

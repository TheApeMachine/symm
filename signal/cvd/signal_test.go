package cvd

import (
	"context"
	"slices"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type staticBookSource struct {
	book *spotbook.Book
}

func (source staticBookSource) Book(_ string, read func(*spotbook.Book)) {
	read(source.book)
}

// seededBook returns a 99.99/100.01 touch so the observed midpoint is 100.0.
func seededBook() *spotbook.Book {
	book := spotbook.New()

	book.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Bid, ID: "bid", Price: decimal.NewFromFloat64(99.99),
		Quantity: decimal.NewFromInt64(40), Timestamp: time.Unix(1, 0).UTC(),
		Silent: true,
	})
	book.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Ask, ID: "ask", Price: decimal.NewFromFloat64(100.01),
		Quantity: decimal.NewFromInt64(10), Timestamp: time.Unix(1, 0).UTC(),
		Silent: true,
	})

	return book
}

func TestMeasure(t *testing.T) {
	Convey("Given CVD trade observations on one symbol", t, func() {
		signal := NewSignal(context.Background(), staticBookSource{book: seededBook()})
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
			So(measurement.Maturity, ShouldEqual, 1.0/flowHistoryCapacity)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 0)
		})
	})

	Convey("Given alternating executions around a constant midpoint", t, func() {
		signal := NewSignal(context.Background(), staticBookSource{book: seededBook()})
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

			for index, measurement := range measurements {
				So(measurement.Sample(types.MetricMidpoint, types.SideNone).Raw, ShouldEqual, 100.0)
				So(measurement.Sample(types.MetricDrive, types.SideNone).Raw, ShouldEqual, 0.0)
				So(measurement.Maturity, ShouldBeGreaterThan, 0)
				So(measurement.Maturity,
					ShouldBeLessThanOrEqualTo, float64(index+1)/flowHistoryCapacity)
			}
		})
	})

	Convey("Given a book advanced past this pass's cut", t, func() {
		// A book whose touch is stamped in the future is beyond the pass cut, so
		// CVD must not respond against that book. The flow conditions against the
		// trade price instead — mirroring the queue-drain stop rule.
		future := seededBook()
		future.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Bid, ID: "bid", Price: decimal.NewFromFloat64(99),
			Quantity: decimal.NewFromInt64(40), Timestamp: time.Now().UTC().Add(time.Hour),
			Silent: true,
		})
		future.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Ask, ID: "ask", Price: decimal.NewFromFloat64(101),
			Quantity: decimal.NewFromInt64(10), Timestamp: time.Now().UTC().Add(time.Hour),
			Silent: true,
		})

		signal := NewSignal(context.Background(), staticBookSource{book: future})
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Now().UTC()
		market.AppendTrade(cvdTrade(9, "buy", 100.5, at), types.TradeReceivers)

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should respond against the trade price, not the future book", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Sample(types.MetricMidpoint, types.SideNone).Raw,
				ShouldEqual, 100.5)
		})
	})
}

func TestFrame(t *testing.T) {
	Convey("Given the equation's first-observation boundary output", t, func() {
		signal := NewSignal(context.Background(), nil)
		at := time.Unix(1_700_003_400, 0).UTC()
		trade := cvdTrade(1, "buy", 100.01, at)

		measurement := signal.frame(
			types.NewSymbol("BTC/USD", nil),
			trade,
			100.0,
			equation.FlowInput{TradeCount: 1},
			equation.FlowOutput{Balance: 0.5, Net: 200.02, NetFraction: 1.0},
		)

		Convey("It should carry the zero scores the boundary left undefined", func() {
			So(measurement.Sample(types.MetricBalance, types.SideNone).Raw, ShouldEqual, 0.5)
			So(measurement.Sample(types.MetricDrive, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricAbsorption, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricStarvation, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 1.0)
			So(measurement.Maturity, ShouldEqual, 1.0/flowHistoryCapacity)
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

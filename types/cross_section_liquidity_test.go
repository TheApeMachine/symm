package types

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func liquidityTestRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       *decimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       *decimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      *decimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

func TestQuoteNotional(t *testing.T) {
	Convey("Given a row with a reported vwap", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 100)

		Convey("When QuoteNotional values it", func() {
			notional := QuoteNotional(row)

			Convey("Then it multiplies volume by vwap, not the mid price", func() {
				So(notional, ShouldEqual, 1000)
			})
		})
	})

	Convey("Given a row with no vwap reported yet", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 0)

		Convey("When QuoteNotional values it", func() {
			notional := QuoteNotional(row)

			Convey("Then it falls back to the last trade price", func() {
				So(notional, ShouldEqual, 10*row.Last.Float64())
			})
		})
	})

	Convey("Given a row with no volume", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 0, 100)

		Convey("When QuoteNotional values it", func() {
			Convey("Then it is zero rather than dividing by an absent quantity", func() {
				So(QuoteNotional(row), ShouldEqual, 0)
			})
		})
	})
}

func TestExecutableDepth(t *testing.T) {
	Convey("Given a two-sided quote with asymmetric quantities", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 5, 2, 10, 100)

		Convey("When ExecutableDepth values it", func() {
			depth := ExecutableDepth(row)

			Convey("Then it uses the smaller side, valued at the mid price", func() {
				So(depth, ShouldEqual, 2*100)
			})
		})
	})

	Convey("Given a one-sided quote with no bid", t, func() {
		row := kraken.TickerData{
			Symbol: "BTC/USD",
			Bid:    *decimal.NewFromFloat64(0),
			BidQty: 0,
			Ask:    *decimal.NewFromFloat64(101),
			AskQty: 5,
		}

		Convey("When ExecutableDepth values it", func() {
			Convey("Then it is zero rather than pricing a one-sided book", func() {
				So(ExecutableDepth(row), ShouldEqual, 0)
			})
		})
	})
}

func TestCrossSectionQuoteNotionalsAndExecutableDepths(t *testing.T) {
	Convey("Given a cross-section observing three symbols", t, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		rows := kraken.TickerDataSlice{
			liquidityTestRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityTestRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityTestRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		}
		So(crossSection.Observe(rows), ShouldBeNil)

		Convey("When QuoteNotionals and ExecutableDepths are read", func() {
			notionals := crossSection.QuoteNotionals()
			depths := crossSection.ExecutableDepths()

			Convey("Then every observed symbol contributes one value to each axis", func() {
				So(notionals, ShouldHaveLength, 3)
				So(depths, ShouldHaveLength, 3)
			})
		})
	})
}

package broker_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestPriceWithFriction(t *testing.T) {
	Convey("Given Kraken's taker fee and an executable quote", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("Trade values should debit the fee on both sides", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Desk.Price().Update(&kraken.TickerData{
				Symbol: symbols[0].Pair,
				Bid:    decimal.NewFromInt64(100),
				Ask:    decimal.NewFromInt64(100),
			})
			volume := decimal.NewFromFloat64(0.4)

			buyValue := market.Desk.Price().WithFriction(symbols[0].Pair, broker.BUY, volume)
			sellValue := market.Desk.Price().WithFriction(symbols[0].Pair, broker.SELL, volume)

			So(buyValue.String(), ShouldEqual, "40.10")
			So(sellValue.String(), ShouldEqual, "39.90")
		}))
	})
}

func TestPriceMark(t *testing.T) {
	Convey("Given Kraken's taker fee and an executable quote", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("A sell mark should debit the liquidation fee", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			market.Desk.Price().Update(&kraken.TickerData{
				Symbol: symbols[0].Pair,
				Bid:    decimal.NewFromInt64(100),
				Ask:    decimal.NewFromInt64(100),
			})

			sellMark := market.Desk.Price().Mark(symbols[0].Pair, broker.SELL)

			So(sellMark.String(), ShouldEqual, "99.74")
		}))
	})
}

func TestPriceFee(t *testing.T) {
	Convey("Given a loaded Kraken fee tier without a cached ticker", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("Fee lookup should not require market data", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			fee, err := market.Desk.Price().Fee(symbols[0].Pair)

			So(err, ShouldBeNil)
			So(fee.String(), ShouldEqual, "0.00260000")
		}))
	})
}

func TestPricePnL(t *testing.T) {
	Convey("Given a flat holding with actual entry and estimated exit fees", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("PnL should retain and debit both fees exactly", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			bid, err := decimal.NewFromString("100.00")
			So(err, ShouldBeNil)
			entryPrice, err := decimal.NewFromString("100.0")
			So(err, ShouldBeNil)
			entryFee, err := decimal.NewFromString("0.10400000")
			So(err, ShouldBeNil)

			market.Desk.Price().Update(&kraken.TickerData{
				Symbol: symbols[0].Pair,
				Bid:    bid,
				Ask:    bid.Copy(),
			})
			holding := &types.Holding{
				Symbol:     symbols[0].Pair,
				Qty:        decimal.NewFromFloat64(0.4),
				EntryPrice: entryPrice,
				EntryFee:   entryFee,
				Mark:       bid.Copy(),
			}

			pair := kraken.InstrumentPair{
				Symbol:        symbols[0].Pair,
				CostPrecision: 5,
			}
			pnl := market.Desk.Price().PnL(pair, holding)

			So(pnl.Float64(), ShouldAlmostEqual, -0.208, 1e-8)
			So(pnl.String(), ShouldEqual, "-0.20800")
		}))
	})
}

package broker

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
)

func TestOrderFactoryBuild(testingTB *testing.T) {
	Convey("Given an order factory with USD balances and a BTC quote", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		factory := NewOrderFactory()
		balances := testBalances(testingTB, "USD", 200)
		ticker := testTicker(testingTB, "BTC/USD", 99, 100, 100)

		Convey("When it builds a market buy from a fraction", func() {
			action := testAction(logic.ActionMarket, logic.SideBuy, "BTC/USD")
			action.Fraction = 0.5

			order, pending, err := factory.Build(action, balances, ticker)

			Convey("Then it creates a Kraken add_order request and pending row", func() {
				So(err, ShouldBeNil)
				So(order.Method, ShouldEqual, "add_order")
				So(order.String("order_type"), ShouldEqual, "market")
				So(order.String("side"), ShouldEqual, "buy")
				quantity, quantityErr := order.Float("order_qty")
				So(quantityErr, ShouldBeNil)
				So(near(quantity, 1), ShouldBeTrue)
				So(pending.ClOrdID, ShouldNotBeBlank)
				So(pending.Symbol, ShouldEqual, "BTC/USD")
				So(pending.Side, ShouldEqual, "buy")
			})
		})

		Convey("When it builds a passive limit buy without an explicit price", func() {
			action := testAction(logic.ActionLimit, logic.SideBuy, "BTC/USD")
			action.Fraction = 0.05

			order, _, err := factory.Build(action, balances, ticker)

			Convey("Then it derives the passive bid limit from the quote", func() {
				So(err, ShouldBeNil)
				limitPrice, limitErr := order.Float("limit_price")
				So(limitErr, ShouldBeNil)
				So(near(limitPrice, 99), ShouldBeTrue)
			})
		})

		Convey("When it builds a market buy from an explicit quantity", func() {
			action := testAction(logic.ActionMarket, logic.SideBuy, "BTC/USD")
			action.Quantity = 2

			order, pending, err := factory.Build(action, balances, ticker)

			Convey("Then it carries the quote notional for batch capital checks", func() {
				So(err, ShouldBeNil)
				quantity, quantityErr := order.Float("order_qty")
				So(quantityErr, ShouldBeNil)
				So(near(quantity, 2), ShouldBeTrue)
				So(near(pending.Notional, 200), ShouldBeTrue)
			})
		})
	})

	Convey("Given an order factory without a BTC quote", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		factory := NewOrderFactory()
		balances := testBalances(testingTB, "USD", 200)
		action := testAction(logic.ActionMarket, logic.SideBuy, "BTC/USD")
		action.Fraction = 0.05

		Convey("When it builds a buy", func() {
			_, _, err := factory.Build(action, balances, NewTicker())

			Convey("Then it rejects the missing quote", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func testBalances(testingTB testing.TB, asset string, balance float64) *BalanceBook {
	testingTB.Helper()

	book := NewBalanceBook()
	frame := map[string]any{
		"channel": "balances",
		"data": []map[string]any{{
			"asset":   asset,
			"balance": balance,
		}},
	}

	if err := book.Update(frame); err != nil {
		testingTB.Fatalf("balance update: %v", err)
	}

	return book
}

func testTicker(
	testingTB testing.TB,
	symbol string,
	bid float64,
	ask float64,
	last float64,
) *Ticker {
	testingTB.Helper()

	ticker := NewTicker()
	frame := map[string]any{
		"channel": "ticker",
		"data": []map[string]any{{
			"symbol": symbol,
			"bid":    bid,
			"ask":    ask,
			"last":   last,
		}},
	}

	if err := ticker.Update(frame); err != nil {
		testingTB.Fatalf("ticker update: %v", err)
	}

	return ticker
}

func testAction(actionType logic.ActionType, side logic.Side, symbol string) *logic.Action {
	return &logic.Action{
		Type:   actionType,
		Side:   side,
		Symbol: symbol,
	}
}

func near(left float64, right float64) bool {
	return math.Abs(left-right) < 1e-9
}

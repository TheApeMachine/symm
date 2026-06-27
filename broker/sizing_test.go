package broker

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func seedBalance(tree *dmt.Tree, quote string, balance float64) {
	artifact := datura.Acquire("kraken:private", datura.APPJSON)
	artifact.WithRole("balances")
	artifact.WithScope("snapshot")
	artifact.SetTimestamp(1)
	artifact.Merge("asset", []map[string]any{
		{"asset": quote, "balance": balance},
	})

	tree.InsertArtifact(artifact.Prefix(), artifact)
}

func seedInstrument(tree *dmt.Tree, symbol, payload string) {
	artifact := datura.Acquire("websocket", datura.APPJSON).
		WithRole("instrument").
		WithScope(symbol).
		WithPayload([]byte(payload))

	tree.InsertArtifact([]byte("instrument/"+symbol+"/"), artifact)
}

func TestDeskAlignsBuyToInstrument(testingTB *testing.T) {
	Convey("Given an instrument with increment and minimums", testingTB, func() {
		viper.Reset()
		viper.Set("market.quote_currency", "USD")
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		seedTicker(tree, "BTC/USD", 100)
		seedBalance(tree, "USD", 1000)
		// fraction 0.10 of 1000 ÷ 100 = 1.07; increment 0.25 rounds down to 1.0.
		seedInstrument(tree, "BTC/USD", `{"qty_increment":0.25,"qty_min":0.1,"cost_min":5}`)

		desk := NewDesk(ctx, pool, tree)

		So(desk.alignEntry("BTC/USD", 1.07, 100), ShouldAlmostEqual, 1.0, 1e-9)

		Convey("Sub-minimum entries are rejected", func() {
			// qty 0.05 < qty_min 0.1 → 0.
			So(desk.alignEntry("BTC/USD", 0.05, 100), ShouldEqual, 0)
		})

		Convey("Sub-cost-min notionals are rejected", func() {
			// qty 0.25 × mark 100 = 25 ≥ cost_min 5, so this passes; but at
			// mark 10 the notional 2.5 < 5 → rejected.
			So(desk.alignEntry("BTC/USD", 0.25, 10), ShouldEqual, 0)
		})

		Convey("Exits round but never reject below minimums", func() {
			// 0.05 is below qty_min, but an exit must still flatten — round only.
			So(desk.roundQuantity("BTC/USD", 0.05), ShouldAlmostEqual, 0.0, 1e-9)
			So(desk.roundQuantity("BTC/USD", 0.6), ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestDeskSizesBuyFromRiskFraction(testingTB *testing.T) {
	Convey("Given a mark, free quote capital, and a risk-fraction entry", testingTB, func() {
		viper.Reset()
		viper.Set("market.quote_currency", "USD")
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		seedTicker(tree, "BTC/USD", 100)
		seedBalance(tree, "USD", 1000)

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)

		// fraction 0.10 of 1000 USD free = 100 USD, priced at mark 100 → qty 1.0.
		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":     "market",
				"fraction": 0.10,
				"offset":   0.05,
			}.Marshal())

		So(desk.Update([]*datura.Artifact{action}), ShouldBeNil)

		Convey("It sizes the order from fraction × free quote ÷ mark", func() {
			order := awaitOrder(orders)

			So(order, ShouldNotBeNil)
			So(datura.Peek[float64](order, "params", "order_qty"), ShouldAlmostEqual, 1.0, 1e-9)
		})
	})
}

func TestDeskSizesBuyAgainstConfiguredSlippage(testingTB *testing.T) {
	Convey("Given configured adverse entry slippage", testingTB, func() {
		viper.Reset()
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.slippage_bps", 100)
		defer viper.Reset()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		seedTicker(tree, "BTC/USD", 100)
		seedBalance(tree, "USD", 1000)

		desk := NewDesk(ctx, pool, tree)

		Convey("It should reserve quote against the worse expected fill", func() {
			// 10% of 1000 USD at a 100 USD mark would be 1 BTC. With 100 bps
			// adverse slippage the desk sizes against 101 USD, so it sends less.
			So(desk.sizeBuy("BTC/USD", 0.10), ShouldAlmostEqual, 100.0/101.0, 1e-9)
		})
	})
}

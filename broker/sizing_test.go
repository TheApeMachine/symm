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

		seedBalance(tree, "USD", 1000)
		// fraction 0.10 of 1000 ÷ 100 = 1.07; increment 0.25 rounds down to 1.0.
		seedInstrument(tree, "BTC/USD", `{"qty_increment":0.25,"qty_min":0.1,"cost_min":5}`)

		desk := NewDesk(ctx, pool, tree)

		qty, qtyErr := desk.alignEntry("BTC/USD", 1.07, 100)
		So(qtyErr, ShouldBeNil)
		So(qty, ShouldAlmostEqual, 1.0, 1e-9)

		Convey("Sub-minimum entries are rejected", func() {
			_, entryErr := desk.alignEntry("BTC/USD", 0.05, 100)
			So(entryErr, ShouldNotBeNil)
		})

		Convey("Sub-cost-min notionals are rejected", func() {
			_, entryErr := desk.alignEntry("BTC/USD", 0.25, 10)
			So(entryErr, ShouldNotBeNil)
		})

		Convey("Exits round to exchange increments", func() {
			_, tinyExitErr := desk.roundQuantity("BTC/USD", 0.05)
			So(tinyExitErr, ShouldNotBeNil)

			rounded, roundErr := desk.roundQuantity("BTC/USD", 0.6)
			So(roundErr, ShouldBeNil)
			So(rounded, ShouldAlmostEqual, 0.5, 1e-9)
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

		seedBalance(tree, "USD", 1000)
		seedInstrument(tree, "BTC/USD", `{"qty_increment":0.000000000001,"qty_min":0.00000001,"cost_min":0.01}`)

		orders := captureOrders(pool)
		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		// fraction 0.10 of 1000 USD free = 100 USD, priced at mark 100 → qty 1.0.
		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope("BTC/USD").
			WithPayload(datura.Map[any]{
				"type":      "market",
				"fraction":  0.10,
				"cl_ord_id": "open-1",
				"offset":    0.05,
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

		seedBalance(tree, "USD", 1000)
		seedInstrument(tree, "BTC/USD", `{"qty_increment":0.000000000001,"qty_min":0.00000001,"cost_min":0.01}`)

		desk := NewDesk(ctx, pool, tree)
		seedTicker(desk, "BTC/USD", 100)

		Convey("It should reserve quote against the worse expected fill", func() {
			// 10% of 1000 USD at a 100 USD mark would be 1 BTC. With 100 bps
			// adverse slippage the desk sizes against 101 USD, so it sends less.
			qty, qtyErr := desk.sizeBuy("BTC/USD", 0.10)
			So(qtyErr, ShouldBeNil)
			So(qty, ShouldAlmostEqual, 100.0/101.0, 1e-9)
		})
	})
}

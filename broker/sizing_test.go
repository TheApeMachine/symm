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

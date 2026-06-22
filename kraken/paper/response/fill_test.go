package response

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

func TestPaperFillUpdatesBalances(testingTB *testing.T) {
	Convey("Given tree quotes and a paper order", testingTB, func() {
		viper.Set("trading.paper.deterministic", true)
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))

		balances := NewBalances(ctx, pool)
		executions := NewExecutions(ctx, pool)
		orders := NewOrdersWithTree(ctx, pool, tree, balances, executions)

		message, buildErr := types.NewKrakenMessage("add_order", map[string]any{
			"order_type": "market",
			"side":       "buy",
			"symbol":     "BTC/USD",
			"order_qty":  0.1,
			"cl_ord_id":  "paper-test",
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(message)

		So(marshalErr, ShouldBeNil)

		Convey("When add_order is routed", func() {
			response := orders.Send(payload)

			So(response, ShouldNotBeNil)
			So(response.Success, ShouldBeTrue)

			time.Sleep(200 * time.Millisecond)

			Convey("Balances should debit quote and credit base", func() {
				usd := assetBalance(balances, "USD")
				btc := assetBalance(balances, "BTC")

				So(usd, ShouldBeLessThan, 200)
				So(btc, ShouldEqual, 0.1)
				So(len(executions.model), ShouldEqual, 1)
			})
		})
	})
}

func TestOrdersScheduleFill(testingTB *testing.T) {
	Convey("Given tree quotes and an add_order payload", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		tree := dmt.NewTree("")

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))

		balances := NewBalances(ctx, nil)
		orders := NewOrdersWithTree(ctx, nil, tree, balances, nil)

		payload, marshalErr := sonic.Marshal(types.KrakenMessage{
			Method: "add_order",
			Params: map[string]any{
				"order_type": "market",
				"side":       "buy",
				"symbol":     "BTC/USD",
				"order_qty":  0.1,
			},
		})

		So(marshalErr, ShouldBeNil)

		order, orderOK := orders.orderFromMessage(payload)

		So(orderOK, ShouldBeTrue)

		orders.scheduleFill(order, "paper-schedule-fill")
		order.Release()

		time.Sleep(200 * time.Millisecond)

		Convey("It should apply the fill to balances", func() {
			So(assetBalance(balances, "BTC"), ShouldEqual, 0.1)
			So(assetBalance(balances, "USD"), ShouldBeLessThan, 200)
		})
	})
}

func TestFillSimulatorSimulate(testingTB *testing.T) {
	Convey("Given ingested ticker quote", testingTB, func() {
		ctx := context.Background()
		tree := dmt.NewTree("")

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))

		fills := NewFillSimulator(ctx, tree)

		order := datura.Acquire("paper", datura.Artifact_Type_json)
		order.Poke("BTC/USD", "symbol")
		order.Poke("buy", "side")
		order.Poke(0.1, "order_qty")
		order.Poke("market", "order_type")

		Convey("Simulate should produce a fill", func() {
			fill, err := fills.Simulate(order, "order-1")

			So(err, ShouldBeNil)
			So(fill, ShouldNotBeNil)
		})
	})
}

func insertIngest(tree *dmt.Tree, role, scope string, payload []byte) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole(role).
		WithScope(scope).
		WithPayload(payload)

	if wire := artifact.Pack(); len(wire) > 0 {
		tree.Insert(artifact.Prefix(), wire)
	}
}

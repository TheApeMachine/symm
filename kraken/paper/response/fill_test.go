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

			time.Sleep(50 * time.Millisecond)

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

func insertIngest(tree *dmt.Tree, role, scope string, payload []byte) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole(role).
		WithScope(scope).
		WithPayload(payload)

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		tree.Insert(artifact.Prefix(), wire)
	}
}

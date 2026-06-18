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
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	. "github.com/theapemachine/symm/signal"
)

func TestPaperFillUpdatesBalances(testingTB *testing.T) {
	Convey("Given tree quotes and a paper order", testingTB, func() {
		viper.Set("trading.paper.deterministic", true)
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := NewTestTree()

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))

		balances := NewBalances(ctx, pool)
		executions := NewExecutions(ctx, pool)
		orders := NewOrdersWithTree(ctx, pool, tree, balances, executions)

		params := trading.AddParams{
			OrderType: trading.Market,
			Side:      trading.Buy,
			Symbol:    "BTC/USD",
			OrderQty:  0.1,
			ClOrdID:   "paper-test",
		}

		message, buildErr := types.NewKrakenMessage(trading.MethodAddOrder, params, 0)

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

func assetBalance(balances *Balances, asset string) float64 {
	if balances == nil {
		return 0
	}

	for _, row := range balances.model.Asset {
		if row.Asset == asset {
			return row.Balance
		}
	}

	return 0
}

func insertIngest(tree *dmt.Tree, role, scope string, payload []byte) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole(role).
		WithScope(scope).
		WithPayload(payload)

	InsertTreeArtifact(tree, artifact)
}

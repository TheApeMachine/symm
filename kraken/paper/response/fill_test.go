package response

import (
	"context"
	"fmt"
	"math"
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
		insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

		balances := NewBalances(ctx, pool, tree)
		executions := NewExecutions(ctx, pool, tree)
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
		insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

		balances := NewBalances(ctx, nil, nil)
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
		insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

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

/*
TestPaperFillAppliesTakerFee proves a market buy pays the real Kraken taker fee
sourced from the AssetPairs schedule, and that the quote balance is debited by
notional plus fee (never just notional, which would be false optimism).
*/
func TestPaperFillAppliesTakerFee(testingTB *testing.T) {
	Convey("Given a ticker quote and a 0.40% taker AssetPairs schedule", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)
		viper.Set("trading.paper.slippage_bps", 0)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))
		insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

		balances := NewBalances(ctx, pool, tree)
		executions := NewExecutions(ctx, pool, tree)
		orders := NewOrdersWithTree(ctx, pool, tree, balances, executions)

		message, buildErr := types.NewKrakenMessage("add_order", map[string]any{
			"order_type": "market",
			"side":       "buy",
			"symbol":     "BTC/USD",
			"order_qty":  0.1,
			"cl_ord_id":  "fee-test",
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(message)

		So(marshalErr, ShouldBeNil)

		Convey("When the order fills", func() {
			response := orders.Send(payload)

			So(response, ShouldNotBeNil)
			So(response.Success, ShouldBeTrue)

			time.Sleep(200 * time.Millisecond)

			Convey("The execution carries fee = price*qty*0.0040 and balances reflect cost+fee", func() {
				So(len(executions.model), ShouldEqual, 1)

				execution := executions.model[0]
				price, _ := execution["last_price"].(float64)
				fee, _ := execution["fee"].(float64)
				feeCcy, _ := execution["fee_ccy"].(string)

				So(price, ShouldBeGreaterThan, 0)
				So(feeCcy, ShouldEqual, "USD")
				So(math.Abs(fee-price*0.1*0.004), ShouldBeLessThan, 1e-9)

				usd := assetBalance(balances, "USD")
				So(math.Abs(usd-(200-price*0.1-fee)), ShouldBeLessThan, 1e-9)
				So(assetBalance(balances, "BTC"), ShouldEqual, 0.1)
			})
		})
	})
}

/*
TestPaperFillRejectsWithoutFeeSchedule proves that with no AssetPairs fee data
for the symbol the order is rejected outright and never fills: pricing a fill
without a real fee is not allowed.
*/
func TestPaperFillRejectsWithoutFeeSchedule(testingTB *testing.T) {
	Convey("Given a ticker quote but no AssetPairs fee schedule", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))

		balances := NewBalances(ctx, pool, tree)
		executions := NewExecutions(ctx, pool, tree)
		orders := NewOrdersWithTree(ctx, pool, tree, balances, executions)

		message, buildErr := types.NewKrakenMessage("add_order", map[string]any{
			"order_type": "market",
			"side":       "buy",
			"symbol":     "BTC/USD",
			"order_qty":  0.1,
			"cl_ord_id":  "no-fee-test",
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(message)

		So(marshalErr, ShouldBeNil)

		Convey("When the order is routed", func() {
			response := orders.Send(payload)

			time.Sleep(200 * time.Millisecond)

			Convey("It is rejected and nothing fills", func() {
				So(response, ShouldNotBeNil)
				So(response.Success, ShouldBeFalse)
				So(assetBalance(balances, "BTC"), ShouldEqual, 0)
				So(assetBalance(balances, "USD"), ShouldEqual, 200)
				So(len(executions.model), ShouldEqual, 0)
			})
		})
	})
}

/*
TestFillConsumesLiveIngestLayout proves the fill path finds a quote stored the
way the live public ingest stores it: the full frame as payload, scope set to the
symbol, and the key role-first with the timestamp behind it. This guards against
the path passing only because tests hand-feed a sim-friendly shape.
*/
func TestFillConsumesLiveIngestLayout(testingTB *testing.T) {
	Convey("Given a ticker keyed exactly as the live ingest keys it", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)
		viper.Set("trading.paper.slippage_bps", 0)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")

		insertLiveFrame(tree, "ticker", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))
		insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

		balances := NewBalances(ctx, pool, tree)
		executions := NewExecutions(ctx, pool, tree)
		orders := NewOrdersWithTree(ctx, pool, tree, balances, executions)

		message, buildErr := types.NewKrakenMessage("add_order", map[string]any{
			"order_type": "market",
			"side":       "buy",
			"symbol":     "BTC/USD",
			"order_qty":  0.1,
			"cl_ord_id":  "live-layout",
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(message)

		So(marshalErr, ShouldBeNil)

		Convey("The order fills against the live-shaped quote", func() {
			response := orders.Send(payload)

			So(response, ShouldNotBeNil)
			So(response.Success, ShouldBeTrue)

			time.Sleep(200 * time.Millisecond)

			So(assetBalance(balances, "BTC"), ShouldEqual, 0.1)
			So(assetBalance(balances, "USD"), ShouldBeLessThan, 200)
			So(len(executions.model), ShouldEqual, 1)
		})
	})
}

// insertLiveFrame seeds the tree exactly as kraken/public ingest does: the full
// frame as payload, scope resolved from data[0].symbol, key role-first.
func insertLiveFrame(tree *dmt.Tree, role string, frame []byte) {
	artifact := datura.Acquire("websocket", datura.Artifact_Type_json).
		WithRole(role).
		WithPayload(frame)

	if symbol := datura.Peek[string](artifact, "data", 0, "symbol"); symbol != "" {
		artifact.WithScope(symbol)
	}

	tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
}

func insertAssetPairs(tree *dmt.Tree, symbol string, taker, maker float64) {
	payload := []byte(fmt.Sprintf(
		`{"wsname":%q,"fees":[[0,%g]],"fees_maker":[[0,%g]],"fee_volume_currency":"ZUSD"}`,
		symbol, taker, maker,
	))

	insertIngest(tree, "assetpairs", symbol, payload)
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

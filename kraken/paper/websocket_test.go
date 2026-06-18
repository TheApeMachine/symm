package paper

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
	"github.com/theapemachine/symm/kraken/user"
	. "github.com/theapemachine/symm/signal"
)

func TestWebSocketOnMessageBalancesSubscribe(t *testing.T) {
	Convey("Given a paper websocket and balances subscribe", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("balances", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		socket := NewWebSocket(ctx, pool, nil)
		go socket.Run()

		request, buildErr := types.NewKrakenMessage("subscribe", user.BalanceParams{
			Channel:  "balances",
			Snapshot: true,
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(request)

		So(marshalErr, ShouldBeNil)

		artifact := datura.Acquire(
			"balances", datura.Artifact_Type_json,
		).WithDestination(
			"kraken:private",
		).WithRole(
			"balances",
		).WithPayload(
			payload,
		)

		destination, destinationErr := artifact.Destination()

		So(destinationErr, ShouldBeNil)
		So(destination, ShouldEqual, "kraken:private")

		Convey("When the subscribe artifact is routed through kraken:private", func() {
			err := pool.CreateBroadcastGroup("kraken:private").Send(artifact)

			Convey("It should deliver a balances snapshot to subscribers", func() {
				So(err, ShouldBeNil)

				var response *datura.Artifact

				select {
				case response = <-received:
				case <-time.After(2 * time.Second):
					So("balances snapshot", ShouldEqual, "received")
				}

				So(datura.Peek[string](response, "role"), ShouldEqual, "balances")
				So(datura.Peek[string](response, "scope"), ShouldEqual, user.BalanceSnapshot)
			})
		})
	})
}

func TestWebSocketFillBroadcastsBalanceUpdate(testingTB *testing.T) {
	Convey("Given a subscribed paper balances channel", testingTB, func() {
		viper.Set("trading.paper.deterministic", true)
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := NewTestTree()

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))

		received := make(chan *datura.Artifact, 2)

		pool.Subscribe("balances", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		socket := NewWebSocket(ctx, pool, tree)
		go socket.Run()

		subscribeRequest, subscribeErr := types.NewKrakenMessage("subscribe", user.BalanceParams{
			Channel:  "balances",
			Snapshot: true,
		}, 0)

		So(subscribeErr, ShouldBeNil)

		subscribePayload, subscribeMarshalErr := sonic.Marshal(subscribeRequest)

		So(subscribeMarshalErr, ShouldBeNil)

		So(pool.CreateBroadcastGroup("kraken:private").Send(
			datura.Acquire("test", datura.Artifact_Type_json).
				WithDestination("kraken:private").
				WithRole("balances").
				WithPayload(subscribePayload),
		), ShouldBeNil)

		select {
		case <-received:
		case <-time.After(2 * time.Second):
			So("balances snapshot", ShouldEqual, "received")
		}

		orderRequest, orderErr := types.NewKrakenMessage(trading.MethodAddOrder, trading.AddParams{
			OrderType: trading.Market,
			Side:      trading.Buy,
			Symbol:    "BTC/USD",
			OrderQty:  0.1,
			ClOrdID:   "paper-fill-broadcast",
		}, 0)

		So(orderErr, ShouldBeNil)

		orderPayload, orderMarshalErr := sonic.Marshal(orderRequest)

		So(orderMarshalErr, ShouldBeNil)

		Convey("When a market order fills", func() {
			So(pool.CreateBroadcastGroup("kraken:private").Send(
				datura.Acquire("test", datura.Artifact_Type_json).
					WithDestination("kraken:private").
					WithRole("orders").
					WithPayload(orderPayload),
			), ShouldBeNil)

			var update *datura.Artifact

			deadline := time.After(2 * time.Second)

			for update == nil {
				select {
				case frame := <-received:
					if datura.Peek[string](frame, "scope") == user.BalanceUpdate {
						update = frame
					}
				case <-deadline:
					So("balances update", ShouldEqual, "received")
				}
			}

			Convey("It should push a balances update frame to subscribers", func() {
				So(datura.Peek[string](update, "role"), ShouldEqual, "balances")
				So(datura.Peek[string](update, "scope"), ShouldEqual, user.BalanceUpdate)

				model := datura.As[user.Balances](update)

				So(walletAssetBalance(model.Asset, "BTC"), ShouldEqual, 0.1)
			})
		})
	})
}

func insertIngest(tree *dmt.Tree, role, scope string, payload []byte) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole(role).
		WithScope(scope).
		WithPayload(payload)

	InsertTreeArtifact(tree, artifact)
}

func walletAssetBalance(rows []user.Balance, asset string) float64 {
	for _, row := range rows {
		if row.Asset == asset {
			return row.Balance
		}
	}

	return 0
}

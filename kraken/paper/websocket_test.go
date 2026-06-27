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
	"github.com/theapemachine/symm/kraken/types"
)

const (
	balanceSnapshotScope = "snapshot"
	balanceUpdateScope   = "update"
)

func TestWebSocketArmsBalancesOnRun(testingTB *testing.T) {
	Convey("Given a paper websocket run loop", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		socket := NewWebSocket(ctx, pool, nil)
		go socket.Run()

		var response *datura.Artifact

		select {
		case response = <-received:
		case <-time.After(2 * time.Second):
			So("balances snapshot", ShouldEqual, "received")
		}

		role, roleErr := response.Role()
		scope, scopeErr := response.Scope()

		So(roleErr, ShouldBeNil)
		So(scopeErr, ShouldBeNil)
		So(role, ShouldEqual, "balances")
		So(scope, ShouldEqual, balanceSnapshotScope)
	})
}

func TestWebSocketOnMessageBalancesSubscribe(t *testing.T) {
	Convey("Given a paper websocket and balances subscribe", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		_ = NewWebSocket(ctx, pool, nil)

		request, buildErr := types.NewKrakenMessage("subscribe", map[string]any{
			"channel":  "balances",
			"snapshot": true,
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

				role, roleErr := response.Role()
				scope, scopeErr := response.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldEqual, balanceSnapshotScope)
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
		tree := dmt.NewTree("")

		insertIngest(tree, "ticker", "BTC/USD", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"bid":99.5,"ask":100.5}]}`,
		))
		insertIngest(tree, "assetpairs", "BTC/USD", []byte(
			`{"wsname":"BTC/USD","fees":[[0,0.4]],"fees_maker":[[0,0.25]],"fee_volume_currency":"ZUSD"}`,
		))

		received := make(chan *datura.Artifact, 2)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		socket := NewWebSocket(ctx, pool, tree)
		go socket.Run()

		select {
		case <-received:
		case <-time.After(2 * time.Second):
			So("balances snapshot", ShouldEqual, "received")
		}

		orderRequest, orderErr := types.NewKrakenMessage("add_order", map[string]any{
			"order_type": "market",
			"side":       "buy",
			"symbol":     "BTC/USD",
			"order_qty":  0.1,
			"cl_ord_id":  "paper-fill-broadcast",
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
					scope, scopeErr := frame.Scope()

					if scopeErr == nil && scope == balanceUpdateScope {
						update = frame
					}
				case <-deadline:
					So("balances update", ShouldEqual, "received")
				}
			}

			Convey("It should push a balances update frame to subscribers", func() {
				role, roleErr := update.Role()
				scope, scopeErr := update.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldEqual, balanceUpdateScope)
				So(walletAssetBalanceFromArtifact(update, "BTC"), ShouldEqual, 0.1)
			})
		})
	})
}

func TestWebSocketPublishesPaperBalancesLikePrivateBus(testingTB *testing.T) {
	Convey("Given a paper websocket handling balances subscribe", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree("")
		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		_ = NewWebSocket(ctx, pool, tree)

		request, buildErr := types.NewKrakenMessage("subscribe", map[string]any{
			"channel":  "balances",
			"snapshot": true,
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(request)

		So(marshalErr, ShouldBeNil)

		err := pool.CreateBroadcastGroup("kraken:private").Send(
			datura.Acquire("test", datura.Artifact_Type_json).
				WithDestination("kraken:private").
				WithRole("balances").
				WithPayload(payload),
		)

		So(err, ShouldBeNil)

		var artifact *datura.Artifact

		select {
		case artifact = <-received:
		case <-time.After(2 * time.Second):
			So("ui balances snapshot", ShouldEqual, "received")
		}

		role, roleErr := artifact.Role()
		scope, scopeErr := artifact.Scope()
		destination, destinationErr := artifact.Destination()

		So(roleErr, ShouldBeNil)
		So(scopeErr, ShouldBeNil)
		So(destinationErr, ShouldBeNil)
		So(role, ShouldEqual, "balances")
		So(scope, ShouldEqual, balanceSnapshotScope)
		So(destination, ShouldEqual, "ui")

		var wire map[string]any

		So(sonic.Unmarshal(artifact.DecryptPayload(), &wire), ShouldBeNil)
		So(wire["asset"], ShouldNotBeNil)
	})
}

func TestWebSocketRunArmsChannels(testingTB *testing.T) {
	Convey("Given a paper websocket", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		socket := NewWebSocket(ctx, pool, nil)

		Convey("Run should arm private channel handlers", func() {
			go socket.Run()
			time.Sleep(50 * time.Millisecond)
			socket.Close()
			So(socket.sockets["balances"] == nil, ShouldBeFalse)
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

func walletAssetBalanceFromArtifact(artifact *datura.Artifact, asset string) float64 {
	if artifact == nil {
		return 0
	}

	if !artifact.HasPayload() {
		return 0
	}

	rawPayload := artifact.DecryptPayload()

	if len(rawPayload) == 0 {
		return 0
	}

	var wire map[string]any

	if sonic.Unmarshal(rawPayload, &wire) != nil {
		return 0
	}

	if _, hasAssetRows := wire["asset"]; !hasAssetRows {
		var envelope types.SocketMessage

		if sonic.Unmarshal(rawPayload, &envelope) == nil {
			if sonic.Unmarshal(envelope.Data, &wire) != nil {
				return 0
			}
		}
	}

	rows, _ := wire["asset"].([]any)

	for _, rowAny := range rows {
		row, ok := rowAny.(map[string]any)

		if !ok {
			continue
		}

		rowAsset, _ := row["asset"].(string)

		if rowAsset != asset {
			continue
		}

		balance, _ := row["balance"].(float64)

		return balance
	}

	return 0
}

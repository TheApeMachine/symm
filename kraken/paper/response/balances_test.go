package response

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

func TestBalancesPublishUpdateRoutesThroughKrakenSocket(testingTB *testing.T) {
	Convey("Given an active balances channel", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("kraken:socket", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		balances := NewBalances(ctx, pool)
		balances.isActive.Store(true)

		Convey("When PublishUpdate is called", func() {
			balances.PublishUpdate()

			Convey("It should route an update frame through kraken:socket", func() {
				var artifact *datura.Artifact

				select {
				case artifact = <-received:
				default:
					So("kraken:socket frame", ShouldEqual, "received")
				}

				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldEqual, balanceUpdateScope)

				payload, payloadErr := artifact.DecryptPayload()

				So(payloadErr, ShouldBeNil)

				var message types.SocketMessage

				So(sonic.Unmarshal(payload, &message), ShouldBeNil)
				So(message.Channel, ShouldEqual, "balances")
				So(message.Type, ShouldEqual, balanceUpdateScope)
				So(message.Success, ShouldBeTrue)
			})
		})
	})
}

func TestBalancesSubscribeSnapshotUsesConfigWallet(testingTB *testing.T) {
	Convey("Given config-backed paper wallet seed", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		balances := NewBalances(ctx, nil)

		request, buildErr := types.NewKrakenMessage("subscribe", map[string]any{
			"channel":  "balances",
			"snapshot": true,
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(request)

		So(marshalErr, ShouldBeNil)

		Convey("When subscribe is handled", func() {
			message := balances.Send(payload)

			Convey("It should emit a snapshot with config quote cash", func() {
				So(message, ShouldNotBeNil)
				So(message.Type, ShouldEqual, balanceSnapshotScope)
				So(assetBalance(balances, "USD"), ShouldEqual, 200)
			})
		})
	})
}

func BenchmarkBalancesPublishUpdate(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)

	balances := NewBalances(ctx, pool)
	balances.isActive.Store(true)

	pool.Subscribe("kraken:socket", func(artifact *datura.Artifact) error {
		return nil
	})

	b.ReportAllocs()

	for b.Loop() {
		balances.PublishUpdate()
	}
}

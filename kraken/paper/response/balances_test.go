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
	"github.com/theapemachine/symm/kraken/user"
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

				So(datura.Peek[string](artifact, "role"), ShouldEqual, "balances")
				So(datura.Peek[string](artifact, "scope"), ShouldEqual, user.BalanceUpdate)

				payload, payloadErr := artifact.DecryptPayload()

				So(payloadErr, ShouldBeNil)

				var message types.SocketMessage

				So(sonic.Unmarshal(payload, &message), ShouldBeNil)
				So(message.Channel, ShouldEqual, "balances")
				So(message.Type, ShouldEqual, user.BalanceUpdate)
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

		request, buildErr := types.NewKrakenMessage("subscribe", user.BalanceParams{
			Channel:  "balances",
			Snapshot: true,
		}, 0)

		So(buildErr, ShouldBeNil)

		payload, marshalErr := sonic.Marshal(request)

		So(marshalErr, ShouldBeNil)

		Convey("When subscribe is handled", func() {
			message := balances.Send(payload)

			Convey("It should emit a snapshot with config quote cash", func() {
				So(message, ShouldNotBeNil)
				So(message.Type, ShouldEqual, user.BalanceSnapshot)

				var model user.Balances

				So(message.Unmarshal(&model), ShouldBeNil)
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

package paper

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
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

		socket := NewWebSocket(ctx, pool)
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
			})
		})
	})
}

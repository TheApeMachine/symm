package public

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func TestPublishStatus(t *testing.T) {
	Convey("Given a status subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		socket := NewWebSocket(ctx, pool)

		received := make(chan string, 1)

		pool.Subscribe("status", func(artifact *datura.Artifact) error {
			received <- datura.Peek[string](artifact, "scope")

			return nil
		})

		Convey("When publishStatus is called", func() {
			socket.publishStatus("connected")

			Convey("It should deliver the scope to subscribers", func() {
				var scope string

				select {
				case scope = <-received:
				case <-time.After(2 * time.Second):
					So("status frame", ShouldEqual, "received")
				}

				So(scope, ShouldEqual, "connected")
			})
		})
	})
}

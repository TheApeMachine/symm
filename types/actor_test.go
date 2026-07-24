package types

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInitializeSize(t *testing.T) {
	Convey("Given a producer and a depth-one consumer", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		Reset(cancel)

		var handled atomic.Int64
		consumer := NewActor(ctx, map[string]Handler{
			"trade": {
				Topic: "trade",
				Fn: func(message any) any {
					handled.Add(1)
					time.Sleep(20 * time.Millisecond)

					return message
				},
			},
		})
		producer := NewActor(ctx, map[string]Handler{
			"trade": {
				Topic: "trade",
				Fn: func(message any) any {
					return message
				},
			},
		})
		producer.AddRoot("trade", NewSubscriptionSize[any](1))
		producer.Run()
		consumer.InitializeSize(1, Topic{Name: "trade", Actor: producer})

		Convey("It should apply backpressure so the consumer sees every send", func() {
			for index := range 5 {
				root := producer.subscriptions["trade"]
				root.Send(index)
			}

			deadline := time.Now().Add(2 * time.Second)

			for handled.Load() < 5 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}

			So(handled.Load(), ShouldEqual, int64(5))
		})
	})
}

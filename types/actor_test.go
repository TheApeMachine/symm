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
					time.Sleep(5 * time.Millisecond)

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
		producer.Start()
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
			So(consumer.Close(), ShouldBeNil)
			So(producer.Close(), ShouldBeNil)
		})
	})
}

func TestActorClose(t *testing.T) {
	Convey("Given a started actor", t, func() {
		ctx := t.Context()
		var ran atomic.Bool
		actor := NewActor(ctx, map[string]Handler{
			"trade": {
				Topic: "trade",
				Fn: func(message any) any {
					ran.Store(true)

					return nil
				},
			},
		})
		root := NewSubscriptionSize[any](1)
		actor.AddRoot("trade", root)
		actor.Start()
		root.Send("ping")

		deadline := time.Now().Add(time.Second)

		for !ran.Load() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		So(ran.Load(), ShouldBeTrue)
		So(actor.Close(), ShouldBeNil)
	})
}

func TestTrySend(t *testing.T) {
	Convey("Given a full subscription", t, func() {
		subscription := NewSubscriptionSize[any](1)
		So(subscription.TrySend(1), ShouldBeTrue)
		So(subscription.TrySend(2), ShouldBeFalse)
	})
}

func BenchmarkActorInbox(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	var handled atomic.Int64
	actor := NewActor(ctx, map[string]Handler{
		"trade": {
			Topic: "trade",
			Fn: func(message any) any {
				handled.Add(1)

				return nil
			},
		},
	})
	root := NewSubscriptionSize[any](1024)
	actor.AddRoot("trade", root)
	actor.Start()
	defer actor.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		root.Send(struct{}{})
	}

	deadline := time.Now().Add(5 * time.Second)

	for handled.Load() < int64(b.N) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
}

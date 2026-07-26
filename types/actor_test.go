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
		consumer := NewActor(ctx, "consumer", map[string]Handler{
			"trade": {
				Topic: "trade",
				Fn: func(message any) any {
					handled.Add(1)
					time.Sleep(5 * time.Millisecond)

					return message
				},
			},
		})
		producer := NewActor(ctx, "producer", map[string]Handler{
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
		actor := NewActor(ctx, "actor", map[string]Handler{
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

func TestRootPublishDoesNotDrop(t *testing.T) {
	Convey("Given a root fan-out with one slow subscriber", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		Reset(cancel)

		live := NewActor(ctx, "live", nil)
		root := NewSubscriptionSize[any](8)
		live.AddRoot("book", root)
		live.Start()

		slow := live.SubscribeSize("book", 1)
		fast := live.SubscribeSize("book", 8)
		So(slow.TrySend("pad"), ShouldBeTrue)

		Convey("It should apply backpressure instead of dropping", func() {
			done := make(chan struct{})
			slowDone := make(chan struct{})

			go func() {
				defer close(slowDone)

				for range 6 {
					<-slow.Channel
					time.Sleep(5 * time.Millisecond)
				}
			}()

			go func() {
				for range 5 {
					root.Send("book")
				}

				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}

			received := 0
			deadline := time.Now().Add(2 * time.Second)

			for received < 5 && time.Now().Before(deadline) {
				select {
				case <-fast.Channel:
					received++
				default:
					time.Sleep(time.Millisecond)
				}
			}

			<-slowDone
			So(received, ShouldEqual, 5)
			So(live.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkActorInbox(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	var handled atomic.Int64
	actor := NewActor(ctx, "actor", map[string]Handler{
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

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
		producer.AddRoot("trade", NewSubscription())
		consumer.Initialize(Topic{Name: "trade", Actor: producer})
		producer.Run()

		Convey("It should apply backpressure so the consumer sees every send", func() {
			for index := range 5 {
				root := producer.subscriptions["trade"][0]
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
		root := NewSubscription()
		actor.AddRoot("trade", root)
		actor.Run()
		root.Send("ping")

		deadline := time.Now().Add(time.Second)

		for !ran.Load() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		So(ran.Load(), ShouldBeTrue)
		So(actor.Close(), ShouldBeNil)
	})
}

func TestSubscriptionSaturationHook(t *testing.T) {
	Convey("Given a subscription with actor edge metadata", t, func() {
		subscription := NewSubscription()
		subscription.edgeKind = "actor"
		subscription.sourceActor = "producer"
		subscription.sourceTopic = "ticker"
		subscription.ownerActor = "consumer"
		subscription.ownerTopic = "ticker"

		captured := make(chan map[string]any, 1)
		SetSubscriptionSaturationHook(func(value map[string]any) {
			captured <- value
		})

		Reset(func() {
			SetSubscriptionSaturationHook(nil)
		})

		for range cap(subscription.Channel) {
			subscription.Channel <- struct{}{}
		}

		Convey("When Send hits a full buffer", func() {
			subscription.Send("payload")

			Convey("Then the hook receives the exact source and owner actor context", func() {
				select {
				case value := <-captured:
					So(value["edgeKind"], ShouldEqual, "actor")
					So(value["sourceActor"], ShouldEqual, "producer")
					So(value["sourceTopic"], ShouldEqual, "ticker")
					So(value["ownerActor"], ShouldEqual, "consumer")
					So(value["ownerTopic"], ShouldEqual, "ticker")
					So(value["bufferCap"], ShouldEqual, cap(subscription.Channel))
					So(value["messageType"], ShouldEqual, "string")
				case <-time.After(time.Second):
					So("timeout", ShouldEqual, "hook invoked")
				}
			})
		})
	})
}

func TestRootPublishDoesNotDrop(t *testing.T) {
	Convey("Given a root fan-out with one slow subscriber", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		Reset(cancel)

		live := NewActor(ctx, "live", nil)
		root := NewSubscription()
		live.AddRoot("book", root)
		live.Run()

		slow := live.Subscribe("book")
		fast := live.Subscribe("book")
		slow.Send("pad")

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

func TestAwaitQuiescence(t *testing.T) {
	Convey("Given a tracked actor chain", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		Reset(cancel)

		producer := NewActor(ctx, "producer", map[string]Handler{
			"ticker": {Topic: "ticker", Fn: func(message any) any { return message }},
		})
		producer.AddRoot("ticker", NewSubscription())

		var handled atomic.Int64
		consumer := NewActor(ctx, "consumer", map[string]Handler{
			"ticker": {
				Topic: "ticker",
				Fn: func(message any) any {
					time.Sleep(5 * time.Millisecond)
					handled.Add(1)
					return nil
				},
			},
		})
		consumer.Initialize(Topic{Name: "ticker", Actor: producer})
		producer.Run()

		Convey("AwaitQuiescence should wait for downstream handling, not just the root send", func() {
			producer.subscriptions["ticker"][0].Send("ping")
			deadline := time.Now().Add(2 * time.Second)

			for handled.Load() < 1 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}

			So(handled.Load(), ShouldEqual, int64(1))
			So(consumer.Close(), ShouldBeNil)
			So(producer.Close(), ShouldBeNil)
		})
	})
}

func TestInitializeSizeSameTopicDifferentActors(t *testing.T) {
	Convey("Given one consumer subscribed to the same topic from two producers", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		Reset(cancel)

		first := NewActor(ctx, "first", map[string]Handler{
			"ticker": {Topic: "ticker", Fn: func(message any) any { return message }},
		})
		second := NewActor(ctx, "second", map[string]Handler{
			"ticker": {Topic: "ticker", Fn: func(message any) any { return message }},
		})
		first.AddRoot("ticker", NewSubscription())
		second.AddRoot("ticker", NewSubscription())
		first.Run()
		second.Run()

		var handled atomic.Int64
		consumer := NewActor(ctx, "consumer", map[string]Handler{
			"ticker": {
				Topic: "ticker",
				Fn: func(message any) any {
					handled.Add(1)
					return nil
				},
			},
		})
		consumer.Initialize(
			Topic{Name: "ticker", Actor: first},
			Topic{Name: "ticker", Actor: second},
		)

		Convey("It pumps both upstream subscriptions instead of overwriting one", func() {
			first.subscriptions["ticker"][0].Send("a")
			second.subscriptions["ticker"][0].Send("b")
			deadline := time.Now().Add(2 * time.Second)

			for handled.Load() < 2 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}

			So(handled.Load(), ShouldEqual, int64(2))
			So(consumer.Close(), ShouldBeNil)
			So(first.Close(), ShouldBeNil)
			So(second.Close(), ShouldBeNil)
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
	root := NewSubscription()
	actor.AddRoot("trade", root)
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

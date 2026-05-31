package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMergeSubscriptionSpecs(t *testing.T) {
	Convey("Given overlapping subscriber specs", t, func() {
		merged := mergeSubscriptionSpecs([]subscriptionSpec{
			{symbols: []string{"ETH/EUR", "BTC/EUR"}},
			{symbols: []string{"SOL/EUR", "BTC/EUR"}, depth: 25},
			{symbols: []string{"ADA/EUR"}, depth: 10},
		})

		Convey("It should union symbols and take the max depth", func() {
			So(merged.symbols, ShouldResemble, []string{"ADA/EUR", "BTC/EUR", "ETH/EUR", "SOL/EUR"})
			So(merged.depth, ShouldEqual, 25)
		})
	})
}

func TestSharedFeedFanoutReliable(t *testing.T) {
	Convey("Given a reliable shared feed with a slow subscriber", t, func() {
		sharedFeed := newReliableSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *int {
			upstream := make(chan *int, 8)

			go func() {
				<-ctx.Done()
				close(upstream)
			}()

			return upstream
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sub := sharedFeed.subscribe(ctx, subscriptionSpec{symbols: []string{"BTC/EUR"}})

		Convey("It should dispatch to subscribers in order without dropping", func() {
			value := 7
			sharedFeed.fanout(&value)

			select {
			case received := <-sub:
				So(received, ShouldNotBeNil)
				So(*received, ShouldEqual, 7)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for async fanout delivery")
			}
		})
	})
}

func TestSharedFeedFanoutReliableOrdering(t *testing.T) {
	Convey("Given two reliable subscribers", t, func() {
		sharedFeed := newReliableSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *int {
			upstream := make(chan *int, 8)

			go func() {
				<-ctx.Done()
				close(upstream)
			}()

			return upstream
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		subOne := sharedFeed.subscribe(ctx, subscriptionSpec{symbols: []string{"BTC/EUR"}})
		subTwo := sharedFeed.subscribe(ctx, subscriptionSpec{symbols: []string{"ETH/EUR"}})

		first := 1
		second := 2
		sharedFeed.fanout(&first)
		sharedFeed.fanout(&second)

		Convey("It should deliver each value to every subscriber in send order", func() {
			So(<-subOne, ShouldEqual, &first)
			So(<-subTwo, ShouldEqual, &first)
			So(<-subOne, ShouldEqual, &second)
			So(<-subTwo, ShouldEqual, &second)
		})
	})
}

func TestSharedFeedDetachDuringReliableFanout(t *testing.T) {
	if !raceDetectorActive() {
		t.Fatal("requires go test -race")
	}

	Convey("Given a reliable feed fanning out while a subscriber detaches", t, func() {
		sharedFeed := newReliableSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *int {
			upstream := make(chan *int, 8)

			go func() {
				<-ctx.Done()
				close(upstream)
			}()

			return upstream
		})

		ctx, cancel := context.WithCancel(context.Background())
		_ = sharedFeed.subscribe(ctx, subscriptionSpec{symbols: []string{"BTC/EUR"}})

		cancel()

		Convey("It should not panic when fanout races with bridge teardown", func() {
			for index := 0; index < 64; index++ {
				value := index
				sharedFeed.fanout(&value)
			}
		})
	})
}

func TestSharedFeedSubscribe(t *testing.T) {
	Convey("Given a shared feed with one upstream", t, func() {
		sharedFeed := newSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *int {
			upstream := make(chan *int, 8)

			go func() {
				<-ctx.Done()
				close(upstream)
			}()

			return upstream
		})

		ctxOne, cancelOne := context.WithCancel(context.Background())
		defer cancelOne()
		ctxTwo, cancelTwo := context.WithCancel(context.Background())
		defer cancelTwo()

		subOne := sharedFeed.subscribe(ctxOne, subscriptionSpec{symbols: []string{"BTC/EUR"}})
		_ = sharedFeed.subscribe(ctxTwo, subscriptionSpec{symbols: []string{"ETH/EUR"}})

		Convey("It should detach and close a subscriber when its context is canceled", func() {
			cancelOne()

			closed := false

			for attempt := 0; attempt < 50 && !closed; attempt++ {
				select {
				case _, ok := <-subOne:
					closed = !ok
				default:
					time.Sleep(time.Millisecond)
				}
			}

			So(closed, ShouldBeTrue)
		})

		Convey("It should stop the upstream when the last subscriber detaches", func() {
			cancelOne()
			cancelTwo()

			stopped := false

			for attempt := 0; attempt < 50 && !stopped; attempt++ {
				sharedFeed.mu.Lock()
				stopped = !sharedFeed.running
				sharedFeed.mu.Unlock()

				if !stopped {
					time.Sleep(time.Millisecond)
				}
			}

			So(stopped, ShouldBeTrue)
		})
	})
}

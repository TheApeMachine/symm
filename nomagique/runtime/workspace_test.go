package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type mockPassNode struct{}

func (node *mockPassNode) Step(input int) int {
	return input * 2
}

func WorkspaceWireTest(t *testing.T) {
	Convey("Given a new Workspace instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When wiring a single subscriber", func() {
			var received atomic.Int64
			var waitGroup sync.WaitGroup
			waitGroup.Add(1)

			workspace.Wire("inbound", "", func(input any) any {
				if value, ok := input.(int64); ok {
					received.Store(value)
					waitGroup.Done()
				}

				return nil
			})

			workspace.Publish("inbound", int64(42))

			waitGroup.Wait()
			So(received.Load(), ShouldEqual, 42)
		})

		Convey("When wiring chained cascading subscribers", func() {
			var stage1Received atomic.Int64
			var stage2Received atomic.Int64
			var stage3Received atomic.Int64
			var waitGroup sync.WaitGroup
			waitGroup.Add(3)

			// Stage 1: "raw" -> "doubled"
			workspace.Wire("raw", "doubled", func(input any) any {
				value := input.(int64)
				stage1Received.Store(value)
				waitGroup.Done()
				return value * 2
			})

			// Stage 2: "doubled" -> "tripled"
			workspace.Wire("doubled", "tripled", func(input any) any {
				value := input.(int64)
				stage2Received.Store(value)
				waitGroup.Done()
				return value * 3
			})

			// Stage 3: "tripled" -> "" (sink)
			workspace.Wire("tripled", "", func(input any) any {
				value := input.(int64)
				stage3Received.Store(value)
				waitGroup.Done()
				return nil
			})

			workspace.Publish("raw", int64(5))

			waitGroup.Wait()
			So(stage1Received.Load(), ShouldEqual, 5)
			So(stage2Received.Load(), ShouldEqual, 10)
			So(stage3Received.Load(), ShouldEqual, 30)
		})

		Convey("When wiring with WireNode and WireFunc helpers", func() {
			var finalOutput atomic.Int64
			var waitGroup sync.WaitGroup
			waitGroup.Add(2)

			node := &mockPassNode{}
			WireNode[int, int](workspace, "node_in", "func_in", node)

			WireFunc[int, int](workspace, "func_in", "", func(input int) int {
				finalOutput.Store(int64(input + 10))
				waitGroup.Done()
				return 0
			})

			// Observer to confirm stage 1 output
			workspace.Observe("func_in", func(topic string, value any) {
				waitGroup.Done()
			})

			workspace.Publish("node_in", 21)

			waitGroup.Wait()
			So(finalOutput.Load(), ShouldEqual, 52)
		})
	})
}

func WorkspaceObserveTest(t *testing.T) {
	Convey("Given a Workspace with Observers registered", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		var topicObserved atomic.Pointer[string]
		var valueObserved atomic.Int64
		var globalCount atomic.Int64
		var waitGroup sync.WaitGroup
		waitGroup.Add(2)

		workspace.Observe("market.ticker", func(topic string, value any) {
			topicObserved.Store(&topic)
			if intVal, ok := value.(int64); ok {
				valueObserved.Store(intVal)
			}
			waitGroup.Done()
		})

		workspace.ObserveAll(func(topic string, value any) {
			globalCount.Add(1)
			waitGroup.Done()
		})

		Convey("When publishing to the observed topic", func() {
			workspace.Publish("market.ticker", int64(100))
			waitGroup.Wait()

			So(*topicObserved.Load(), ShouldEqual, "market.ticker")
			So(valueObserved.Load(), ShouldEqual, 100)
			So(globalCount.Load(), ShouldEqual, 1)
		})
	})
}

func WorkspaceShareTest(t *testing.T) {
	Convey("Given a Workspace shared object pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When sharing and retrieving objects with composite IDs", func() {
			workspace.Share("orderbook", "BTC_BOOK", "BTC/USD")
			workspace.Share("orderbook", "ETH_BOOK", "ETH/USD")
			workspace.Share("thesis", 12345)

			btcVal, foundBTC := workspace.Shared("orderbook", "BTC/USD")
			ethVal, foundETH := workspace.Shared("orderbook", "ETH/USD")
			thesisVal, foundThesis := workspace.Shared("thesis")
			missingVal, foundMissing := workspace.Shared("nonexistent")

			So(foundBTC, ShouldBeTrue)
			So(btcVal, ShouldEqual, "BTC_BOOK")

			So(foundETH, ShouldBeTrue)
			So(ethVal, ShouldEqual, "ETH_BOOK")

			So(foundThesis, ShouldBeTrue)
			So(thesisVal, ShouldEqual, 12345)

			So(foundMissing, ShouldBeFalse)
			So(missingVal, ShouldBeNil)
		})

		Convey("When sharing with tricky delimiters to ensure no collisions", func() {
			workspace.Share("a:b", "first", "c")
			workspace.Share("a", "second", "b:c")

			valA, foundA := workspace.Shared("a:b", "c")
			valB, foundB := workspace.Shared("a", "b:c")

			So(foundA, ShouldBeTrue)
			So(valA, ShouldEqual, "first")

			So(foundB, ShouldBeTrue)
			So(valB, ShouldEqual, "second")
		})
	})
}

func WorkspaceOnTest(t *testing.T) {
	Convey("Given a Workspace signalling layer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When registering multiple listeners on a signal", func() {
			var triggerCount atomic.Int64
			const listenerCount = 10
			var waitGroup sync.WaitGroup
			waitGroup.Add(listenerCount)

			for index := 0; index < listenerCount; index++ {
				workspace.On("websocket.disconnected", func() {
					triggerCount.Add(1)
					waitGroup.Done()
				})
			}

			workspace.Notify("websocket.disconnected")

			waitGroup.Wait()
			So(triggerCount.Load(), ShouldEqual, listenerCount)
		})
	})
}

func TestWorkspace(t *testing.T) {
	WorkspaceWireTest(t)
	WorkspaceObserveTest(t)
	WorkspaceShareTest(t)
	WorkspaceOnTest(t)
}

func BenchmarkWorkspacePublish(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	workspace := NewWorkspace(ctx)

	defer func() {
		workspace.Close()
		cancel()
	}()

	var counter atomic.Int64
	workspace.Observe("bench", func(topic string, value any) {
		counter.Add(1)
	})

	b.ResetTimer()
	b.ReportAllocs()

	for index := 0; index < b.N; index++ {
		workspace.Publish("bench", int64(1))
	}

	// Allow pending messages to drain
	time.Sleep(10 * time.Millisecond)
}

package runtime

import (
	"context"
	"fmt"
	"runtime"
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

type testKeyedEvent struct {
	Symbol string
	Seq    int
}

func (event testKeyedEvent) ExecutionKey() string {
	return event.Symbol
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

			done := make(chan struct{})
			go func() {
				waitGroup.Wait()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("subscriber did not receive published value")
			}

			So(received.Load(), ShouldEqual, 42)
		})

		Convey("When wiring chained cascading subscribers", func() {
			var stage1Received atomic.Int64
			var stage2Received atomic.Int64
			var stage3Received atomic.Int64
			var waitGroup sync.WaitGroup
			waitGroup.Add(3)

			workspace.Wire("raw", "doubled", func(input any) any {
				value := input.(int64)
				stage1Received.Store(value)
				waitGroup.Done()
				return value * 2
			})

			workspace.Wire("doubled", "tripled", func(input any) any {
				value := input.(int64)
				stage2Received.Store(value)
				waitGroup.Done()
				return value * 3
			})

			workspace.Wire("tripled", "", func(input any) any {
				value := input.(int64)
				stage3Received.Store(value)
				waitGroup.Done()
				return nil
			})

			workspace.Publish("raw", int64(5))

			done := make(chan struct{})
			go func() {
				waitGroup.Wait()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("cascaded subscribers did not complete")
			}

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

			workspace.Wire("func_in", "", func(_ any) any {
				waitGroup.Done()
				return nil
			})

			workspace.Publish("node_in", 21)

			done := make(chan struct{})
			go func() {
				waitGroup.Wait()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("node/func helpers did not complete")
			}

			So(finalOutput.Load(), ShouldEqual, 52)
		})

		Convey("When publishing high volume cascading events across multiple stages", func() {
			var stage3Received atomic.Int64
			const totalEvents = 2000

			workspace.Wire("stage1", "stage2", func(input any) any {
				return input.(int64) + 1
			})

			workspace.Wire("stage2", "stage3", func(input any) any {
				return input.(int64) * 2
			})

			workspace.Wire("stage3", "", func(input any) any {
				stage3Received.Add(1)
				return nil
			})

			for index := int64(0); index < totalEvents; index++ {
				workspace.Publish("stage1", index)
			}

			err := workspace.WaitForQuiescence()
			So(err, ShouldBeNil)
			So(stage3Received.Load(), ShouldEqual, totalEvents)
		})
	})
}

func WorkspaceConcurrencyTest(t *testing.T) {
	Convey("Given a Workspace with parallel key-affine handlers", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When publishing across many unique symbols, different symbols execute concurrently", func() {
			const symbolCount = 640
			var activeCount atomic.Int64
			var maxConcurrent atomic.Int64
			var completedCount atomic.Int64

			WireKeyed[testKeyedEvent, any](
				workspace,
				"symbol_stream",
				"",
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) any {
					current := activeCount.Add(1)

					for {
						highest := maxConcurrent.Load()
						if current <= highest || maxConcurrent.CompareAndSwap(highest, current) {
							break
						}
					}

					time.Sleep(2 * time.Millisecond)
					activeCount.Add(-1)
					completedCount.Add(1)
					return nil
				},
			)

			for index := 0; index < symbolCount; index++ {
				workspace.Publish("symbol_stream", testKeyedEvent{
					Symbol: fmt.Sprintf("SYM-%d", index),
					Seq:    index,
				})
			}

			err := workspace.WaitForQuiescence(10 * time.Second)
			So(err, ShouldBeNil)
			So(completedCount.Load(), ShouldEqual, symbolCount)

			if runtime.GOMAXPROCS(0) > 1 {
				So(maxConcurrent.Load(), ShouldBeGreaterThan, 1)
			}
		})

		Convey("When publishing many events for the same symbol, the same symbol never executes concurrently", func() {
			const eventCount = 200
			var activeCount atomic.Int64
			var maxConcurrent atomic.Int64
			var completedCount atomic.Int64

			WireKeyed[testKeyedEvent, any](
				workspace,
				"single_symbol_stream",
				"",
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) any {
					current := activeCount.Add(1)

					for {
						highest := maxConcurrent.Load()
						if current <= highest || maxConcurrent.CompareAndSwap(highest, current) {
							break
						}
					}

					time.Sleep(100 * time.Microsecond)
					activeCount.Add(-1)
					completedCount.Add(1)
					return nil
				},
			)

			for index := 0; index < eventCount; index++ {
				workspace.Publish("single_symbol_stream", testKeyedEvent{
					Symbol: "BTC/USD",
					Seq:    index,
				})
			}

			err := workspace.WaitForQuiescence(5 * time.Second)
			So(err, ShouldBeNil)
			So(completedCount.Load(), ShouldEqual, eventCount)
			So(maxConcurrent.Load(), ShouldEqual, 1)
		})

		Convey("When publishing sequential events for one symbol, same-symbol ordering is exact", func() {
			const eventCount = 10000
			expectedSeq := int64(0)
			var sequenceMismatch atomic.Bool
			var completedCount atomic.Int64

			WireKeyed[testKeyedEvent, any](
				workspace,
				"ordered_stream",
				"",
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) any {
					if int64(event.Seq) != expectedSeq {
						sequenceMismatch.Store(true)
					}

					expectedSeq++
					completedCount.Add(1)
					return nil
				},
			)

			for index := 0; index < eventCount; index++ {
				workspace.Publish("ordered_stream", testKeyedEvent{
					Symbol: "BTC/USD",
					Seq:    index,
				})
			}

			err := workspace.WaitForQuiescence(10 * time.Second)
			So(err, ShouldBeNil)
			So(completedCount.Load(), ShouldEqual, eventCount)
			So(sequenceMismatch.Load(), ShouldBeFalse)
		})

		Convey("When one symbol is slow, other symbols progress without blocking", func() {
			var fastCount atomic.Int64
			var slowStarted atomic.Bool
			var slowFinished atomic.Bool

			WireKeyed[testKeyedEvent, any](
				workspace,
				"multi_symbol_stream",
				"",
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) any {
					if event.Symbol == "SLOW" {
						slowStarted.Store(true)
						time.Sleep(100 * time.Millisecond)
						slowFinished.Store(true)
						return nil
					}

					fastCount.Add(1)
					return nil
				},
			)

			workspace.Publish("multi_symbol_stream", testKeyedEvent{Symbol: "SLOW", Seq: 1})

			for !slowStarted.Load() {
				time.Sleep(1 * time.Millisecond)
			}

			// Publish many fast symbols across distinct keys; at least one must
			// map to a handler other than SLOW's and therefore complete while
			// SLOW is still blocked. This asserts cross-key concurrency without
			// requiring every key to land on a different lane.
			for index := 0; index < 200; index++ {
				workspace.Publish("multi_symbol_stream", testKeyedEvent{
					Symbol: fmt.Sprintf("FAST-%d", index),
					Seq:    index,
				})
			}

			if runtime.GOMAXPROCS(0) > 1 {
				eventually(t, func() bool {
					return fastCount.Load() > 0 && !slowFinished.Load()
				})
			}

			err := workspace.WaitForQuiescence(3 * time.Second)
			So(err, ShouldBeNil)
			So(slowFinished.Load(), ShouldBeTrue)
		})

		Convey("When sustained high-volume workload runs, goroutine count remains bounded", func() {
			const eventCount = 10000
			initialGoroutines := runtime.NumGoroutine()

			WireKeyed[testKeyedEvent, any](
				workspace,
				"bounded_goroutine_stream",
				"",
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) any {
					return nil
				},
			)

			for index := 0; index < eventCount; index++ {
				workspace.Publish("bounded_goroutine_stream", testKeyedEvent{
					Symbol: fmt.Sprintf("SYM-%d", index%100),
					Seq:    index,
				})
			}

			err := workspace.WaitForQuiescence(10 * time.Second)
			So(err, ShouldBeNil)

			currentGoroutines := runtime.NumGoroutine()
			So(currentGoroutines-initialGoroutines, ShouldBeLessThan, 100)
		})

		Convey("When inspecting subscriber construction, one Disruptor exists per logical subscriber", func() {
			WireKeyed[testKeyedEvent, any](
				workspace,
				"one_ring_stream",
				"",
				func(event testKeyedEvent) string { return event.Symbol },
				func(event testKeyedEvent) any { return nil },
			)

			snapshots := workspace.Snapshots()
			So(len(snapshots), ShouldEqual, 1)

			snapshot := snapshots[0]
			So(snapshot.Capacity, ShouldEqual, uint64(subscriberCapacity))
			So(snapshot.HandlerCount, ShouldEqual, workspace.handlerCount)

			// Capacity must not be multiplied by handler count: a physical ring
			// cannot contain more than its capacity.
			So(snapshot.Capacity, ShouldBeLessThanOrEqualTo, uint64(subscriberCapacity))

			// Adversarial: introspect the actual registered subscriber, not the
			// snapshot's self-report. There must be exactly one logical
			// subscriber with exactly one physical Disruptor and one contiguous
			// ring buffer of exactly the declared capacity, while the parallel
			// handler count is an invariant of machine capacity — never an
			// extra ring.
			workspace.subscribersMu.RLock()
			subscribers := workspace.subscribers
			workspace.subscribersMu.RUnlock()

			So(len(subscribers), ShouldEqual, 1)

			subscriber := subscribers[0]
			So(subscriber, ShouldNotBeNil)
			So(subscriber.disruptor, ShouldNotBeNil)
			So(len(subscriber.handlers), ShouldEqual, workspace.handlerCount)
			So(len(subscriber.buffer), ShouldEqual, int(subscriberCapacity))
			So(subscriber.handlerCnt, ShouldEqual, workspace.handlerCount)
		})
	})
}

func WorkspaceServiceClassTest(t *testing.T) {
	Convey("Given analytics saturation", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("UI-by-pass: UI consumes latest state while every analytics permit is held", func() {
			var uiReceived atomic.Int64

			// Saturate the analytics semaphore with CPU-heavy Steps that hold
			// their permit until released.
			release := make(chan struct{})
			wireAnalyticsBlocker(workspace, "saturated_analytics", release)

			// Wait until at least one analytics Step has entered and is blocked.
			eventually(t, func() bool {
				snap := workspace.TopicSnapshot("saturated_analytics")
				return snap.ActiveHandlers > 0 || len(workspace.analyticsSem) > 0
			})

			workspace.WireClass(
				"ui_state",
				"",
				ServiceUI,
				DeliveryLatestByKey,
				func(any) string { return "global" },
				func(any) any {
					uiReceived.Add(1)
					return nil
				},
			)

			workspace.Publish("ui_state", testKeyedEvent{Symbol: "g", Seq: 1})

			eventually(t, func() bool {
				return uiReceived.Load() >= 1
			})

			close(release)
		})

		Convey("Priority bypass: PositionGuardian-style priority subscriber consumes without an analytics permit", func() {
			var priorityReceived atomic.Int64
			release := make(chan struct{})
			wireAnalyticsBlocker(workspace, "saturated_priority_analytics", release)

			eventually(t, func() bool {
				return len(workspace.analyticsSem) > 0
			})

			workspace.WireClass(
				"priority_stream",
				"",
				ServicePriorityControl,
				DeliveryPriorityFIFO,
				func(any) string { return "global" },
				func(any) any {
					priorityReceived.Add(1)
					return nil
				},
			)

			workspace.Publish("priority_stream", testKeyedEvent{Symbol: "p", Seq: 1})

			eventually(t, func() bool {
				return priorityReceived.Load() >= 1
			})

			close(release)
		})
	})
}

func wireAnalyticsBlocker(workspace *Workspace, topic string, release chan struct{}) {
	workspace.WireClass(
		topic,
		"",
		ServiceAnalytics,
		DeliveryReliableFIFO,
		func(any) string { return "global" },
		func(any) any {
			<-release
			return nil
		},
	)

	workspace.Publish(topic, testKeyedEvent{Symbol: "block", Seq: 1})
}

func WorkspaceObservationalTest(t *testing.T) {
	Convey("Given an observational subscriber whose consumer never advances", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When TryReserve runs out of capacity, the drop is exact, explicit, and non-blocking", func() {
			// A consumer that never completes any event leaves the ring full at
			// its physical capacity. Once full, every further publication must
			// fail TryReserve promptly and increment the drop counter exactly
			// once per failed publication — it must never block and never creep
			// past the ring's physical slot count.
			workspace.WireClass(
				"obs_stream",
				"",
				ServiceAnalytics,
				DeliveryObservationalFIFO,
				func(any) string { return globalKey },
				func(any) any {
					// Never complete: block until the workspace cancels, so the
					// consumer's sequence stays pinned and the ring cannot refill.
					<-workspace.ctx.Done()
					return nil
				},
			)

			// Publish far more than the ring can hold. Every publication is a
			// TryReserve; the first capacity's worth are accepted and committed,
			// the remainder must fail non-blockingly and be counted.
			const capacity = int(subscriberCapacity)
			const extra = 100

			publishStarted := time.Now()

			for index := 0; index < capacity+extra; index++ {
				workspace.Publish("obs_stream", index)
			}

			elapsed := time.Since(publishStarted)

			// Non-blocking: publishing 65,636 attempts must finish fast.
			So(elapsed, ShouldBeLessThan, 2*time.Second)

			subscriber := workspace.subscribers[0]
			So(subscriber, ShouldNotBeNil)

			// Exactly one physical ring of exactly the declared capacity.
			So(subscriber.dropped.Load(), ShouldEqual, uint64(extra))
			So(subscriber.tryReserveFmt.Load(), ShouldEqual, uint64(extra))
			So(subscriber.published.Load(), ShouldEqual, uint64(capacity))
			So(len(subscriber.buffer), ShouldEqual, capacity)
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

			done := make(chan struct{})
			go func() {
				waitGroup.Wait()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("signal listeners did not fire")
			}

			So(triggerCount.Load(), ShouldEqual, listenerCount)
		})
	})
}

func TestWorkspace(t *testing.T) {
	WorkspaceWireTest(t)
	WorkspaceConcurrencyTest(t)
	WorkspaceServiceClassTest(t)
	WorkspaceObservationalTest(t)
	WorkspaceShareTest(t)
	WorkspaceOnTest(t)
}

/*
eventually polls until the condition is true or the test times out, so
concurrency assertions do not rely on arbitrary sleeps.
*/
func eventually(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("condition did not become true within deadline")
}

func BenchmarkWorkspacePublish(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	workspace := NewWorkspace(ctx)

	defer func() {
		workspace.Close()
		cancel()
	}()

	var counter atomic.Int64
	workspace.Wire("bench", "", func(_ any) any {
		counter.Add(1)
		return nil
	})

	b.ReportAllocs()

	for b.Loop() {
		workspace.Publish("bench", int64(1))
	}

	_ = workspace.WaitForQuiescence(5 * time.Second)
}

func BenchmarkWorkspace640Symbols(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	workspace := NewWorkspace(ctx)

	defer func() {
		workspace.Close()
		cancel()
	}()

	const symbolCount = 640
	symbols := make([]string, symbolCount)

	for index := 0; index < symbolCount; index++ {
		symbols[index] = fmt.Sprintf("SYM_%d", index)
	}

	var processedCount atomic.Int64
	WireKeyed[testKeyedEvent, any](
		workspace,
		"bench_640",
		"",
		func(event testKeyedEvent) string {
			return event.Symbol
		},
		func(event testKeyedEvent) any {
			processedCount.Add(1)
			return nil
		},
	)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for index := 0; index < symbolCount; index++ {
			workspace.Publish("bench_640", testKeyedEvent{
				Symbol: symbols[index],
				Seq:    index,
			})
		}
	}

	_ = workspace.WaitForQuiescence(10 * time.Second)
}

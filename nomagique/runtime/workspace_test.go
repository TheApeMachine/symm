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

// Distinct wire types stand in for the old topic strings: routing is by Go
// type now, so a pipeline stage needs its own type the same way it used to
// need its own topic name.
type inboundValue int64
type rawValue int64
type doubledValue int64
type tripledValue int64
type nodeInValue int
type funcInValue int
type stage1Value int64
type stage2Value int64
type stage3Value int64

func WorkspaceWireTest(t *testing.T) {
	Convey("Given a new Workspace instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)
		feed := workspace.NewFeed()

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When wiring a single subscriber", func() {
			var received atomic.Int64
			var waitGroup sync.WaitGroup
			waitGroup.Add(1)

			RegisterSink(workspace, nil, func(value inboundValue) {
				received.Store(int64(value))
				waitGroup.Done()
			})

			feed.Emit(inboundValue(42))

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

			Register(workspace, nil, func(value rawValue) doubledValue {
				stage1Received.Store(int64(value))
				waitGroup.Done()
				return doubledValue(value * 2)
			})

			Register(workspace, nil, func(value doubledValue) tripledValue {
				stage2Received.Store(int64(value))
				waitGroup.Done()
				return tripledValue(value * 3)
			})

			RegisterSink(workspace, nil, func(value tripledValue) {
				stage3Received.Store(int64(value))
				waitGroup.Done()
			})

			feed.Emit(rawValue(5))

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

		Convey("When wiring with a Node and a plain Register step", func() {
			var finalOutput atomic.Int64
			var waitGroup sync.WaitGroup
			waitGroup.Add(2)

			node := &mockPassNode{}
			Register(workspace, nil, func(value nodeInValue) funcInValue {
				return funcInValue(node.Step(int(value)))
			})

			RegisterSink(workspace, nil, func(value funcInValue) {
				finalOutput.Store(int64(value) + 10)
				waitGroup.Done()
			})

			RegisterSink(workspace, nil, func(value funcInValue) {
				waitGroup.Done()
			})

			feed.Emit(nodeInValue(21))

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

			Register(workspace, nil, func(value stage1Value) stage2Value {
				return stage2Value(value + 1)
			})

			Register(workspace, nil, func(value stage2Value) stage3Value {
				return stage3Value(value * 2)
			})

			RegisterSink(workspace, nil, func(value stage3Value) {
				stage3Received.Add(1)
			})

			for index := int64(0); index < totalEvents; index++ {
				feed.Emit(stage1Value(index))
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
		feed := workspace.NewFeed()

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When publishing across many unique symbols, different symbols execute concurrently", func() {
			const symbolCount = 640
			var activeCount atomic.Int64
			var maxConcurrent atomic.Int64
			var completedCount atomic.Int64

			RegisterSink(
				workspace,
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) {
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
				},
			)

			for index := 0; index < symbolCount; index++ {
				feed.Emit(testKeyedEvent{
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

			RegisterSink(
				workspace,
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) {
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
				},
			)

			for index := 0; index < eventCount; index++ {
				feed.Emit(testKeyedEvent{
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

			RegisterSink(
				workspace,
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) {
					if int64(event.Seq) != expectedSeq {
						sequenceMismatch.Store(true)
					}

					expectedSeq++
					completedCount.Add(1)
				},
			)

			for index := 0; index < eventCount; index++ {
				feed.Emit(testKeyedEvent{
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

			RegisterSink(
				workspace,
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) {
					if event.Symbol == "SLOW" {
						slowStarted.Store(true)
						time.Sleep(100 * time.Millisecond)
						slowFinished.Store(true)
						return
					}

					fastCount.Add(1)
				},
			)

			feed.Emit(testKeyedEvent{Symbol: "SLOW", Seq: 1})

			for !slowStarted.Load() {
				time.Sleep(1 * time.Millisecond)
			}

			// Publish many fast symbols across distinct keys; at least one must
			// map to a handler other than SLOW's and therefore complete while
			// SLOW is still blocked. This asserts cross-key concurrency without
			// requiring every key to land on a different lane.
			for index := 0; index < 200; index++ {
				feed.Emit(testKeyedEvent{
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

			RegisterSink(
				workspace,
				func(event testKeyedEvent) string {
					return event.Symbol
				},
				func(event testKeyedEvent) {},
			)

			for index := 0; index < eventCount; index++ {
				feed.Emit(testKeyedEvent{
					Symbol: fmt.Sprintf("SYM-%d", index%100),
					Seq:    index,
				})
			}

			err := workspace.WaitForQuiescence(10 * time.Second)
			So(err, ShouldBeNil)

			currentGoroutines := runtime.NumGoroutine()
			So(currentGoroutines-initialGoroutines, ShouldBeLessThan, 100)
		})

		Convey("When inspecting subscriber construction, one Disruptor ring exists per producer-consumer edge", func() {
			RegisterSink(
				workspace,
				func(event testKeyedEvent) string { return event.Symbol },
				func(event testKeyedEvent) {},
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
			// consumer node; publishing from this test's one Feed identity
			// creates exactly one physical ring (one producer, one consumer),
			// with a contiguous ring buffer of exactly the declared capacity,
			// while the parallel handler count is an invariant of machine
			// capacity — never an extra ring.
			workspace.subscribersMu.RLock()
			subscribers := workspace.subscribers
			workspace.subscribersMu.RUnlock()

			So(len(subscribers), ShouldEqual, 1)

			subscriber := subscribers[0]
			So(subscriber, ShouldNotBeNil)
			So(subscriber.handlerCnt, ShouldEqual, workspace.handlerCount)

			feed.Emit(testKeyedEvent{Symbol: "ring-probe", Seq: 1})
			err := workspace.WaitForQuiescence(2 * time.Second)
			So(err, ShouldBeNil)

			loaded := subscriber.rings.Load()
			So(loaded, ShouldNotBeNil)
			So(len(*loaded), ShouldEqual, 1)

			for _, target := range *loaded {
				So(target, ShouldNotBeNil)
				So(target.disruptor, ShouldNotBeNil)
				So(len(target.handlers), ShouldEqual, workspace.handlerCount)
				So(len(target.buffer), ShouldEqual, int(subscriberCapacity))
			}
		})

		Convey("When two producers feed the same consumer type, each gets its own dedicated ring", func() {
			var completedCount atomic.Int64

			RegisterSink(
				workspace,
				func(event testKeyedEvent) string { return event.Symbol },
				func(event testKeyedEvent) { completedCount.Add(1) },
			)

			feedA := workspace.NewFeed()
			feedB := workspace.NewFeed()

			for index := 0; index < 50; index++ {
				feedA.Emit(testKeyedEvent{Symbol: fmt.Sprintf("A-%d", index), Seq: index})
				feedB.Emit(testKeyedEvent{Symbol: fmt.Sprintf("B-%d", index), Seq: index})
			}

			err := workspace.WaitForQuiescence(5 * time.Second)
			So(err, ShouldBeNil)
			So(completedCount.Load(), ShouldEqual, 100)

			workspace.subscribersMu.RLock()
			subscribers := workspace.subscribers
			workspace.subscribersMu.RUnlock()

			So(len(subscribers), ShouldEqual, 1)

			loaded := subscribers[0].rings.Load()
			So(loaded, ShouldNotBeNil)
			// One dedicated ring per distinct Feed identity that actually wrote
			// to this consumer: never a ring shared between feedA and feedB.
			So(len(*loaded), ShouldEqual, 2)
		})
	})
}

func WorkspaceServiceClassTest(t *testing.T) {
	Convey("Given analytics saturation", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)
		feed := workspace.NewFeed()

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("UI-by-pass: UI consumes latest state while every analytics permit is held", func() {
			var uiReceived atomic.Int64

			// Saturate the analytics semaphore with CPU-heavy Steps that hold
			// their permit until released.
			release := make(chan struct{})
			blockerFeed := wireAnalyticsBlocker(workspace, release)

			// Wait until at least one analytics Step has entered and is blocked.
			eventually(t, func() bool {
				snap := TypeSnapshot[analyticsBlockerValue](workspace)
				return snap.ActiveHandlers > 0 || len(workspace.analyticsSem) > 0
			})

			RegisterSinkClass(
				workspace,
				ServiceUI,
				DeliveryLatestByKey,
				func(uiStateValue) string { return "global" },
				func(uiStateValue) {
					uiReceived.Add(1)
				},
			)

			_ = blockerFeed
			feed.Emit(uiStateValue{Symbol: "g", Seq: 1})

			eventually(t, func() bool {
				return uiReceived.Load() >= 1
			})

			close(release)
		})

		Convey("Priority bypass: PositionGuardian-style priority subscriber consumes without an analytics permit", func() {
			var priorityReceived atomic.Int64
			release := make(chan struct{})
			wireAnalyticsBlocker(workspace, release)

			eventually(t, func() bool {
				return len(workspace.analyticsSem) > 0
			})

			RegisterSinkClass(
				workspace,
				ServicePriorityControl,
				DeliveryPriorityFIFO,
				func(priorityStreamValue) string { return "global" },
				func(priorityStreamValue) {
					priorityReceived.Add(1)
				},
			)

			feed.Emit(priorityStreamValue{Symbol: "p", Seq: 1})

			eventually(t, func() bool {
				return priorityReceived.Load() >= 1
			})

			close(release)
		})

		Convey("A panicking analytics Step must not leak its permit", func() {
			var panicsEntered atomic.Int64

			RegisterSinkClass(
				workspace,
				ServiceAnalytics,
				DeliveryReliableFIFO,
				func(panicAnalyticsValue) string { return "global" },
				func(panicAnalyticsValue) {
					panicsEntered.Add(1)
					panic("boom")
				},
			)

			// Saturate every permit with panics. If a panic leaked its permit,
			// the analytics semaphore would stay full forever and the healthy
			// analytics subscriber below would starve.
			for index := 0; index < workspace.handlerCount; index++ {
				feed.Emit(panicAnalyticsValue{Symbol: "panic", Seq: index})
			}

			// Every panic must have entered AND fully unwound: the semaphore
			// drains to empty (released) and no handler is mid-batch.
			eventually(t, func() bool {
				if panicsEntered.Load() < int64(workspace.handlerCount) {
					return false
				}

				return len(workspace.analyticsSem) == 0 &&
					TypeSnapshot[panicAnalyticsValue](workspace).ActiveHandlers == 0
			})

			// A healthy analytics subscriber must still be able to acquire a
			// permit and complete, proving no permit was leaked.
			var healthyReceived atomic.Int64

			RegisterSinkClass(
				workspace,
				ServiceAnalytics,
				DeliveryReliableFIFO,
				func(healthyAnalyticsValue) string { return "global" },
				func(healthyAnalyticsValue) {
					healthyReceived.Add(1)
				},
			)

			feed.Emit(healthyAnalyticsValue{Symbol: "healthy", Seq: 1})

			eventually(t, func() bool {
				return healthyReceived.Load() >= 1
			})
		})
	})
}

type analyticsBlockerValue testKeyedEvent
type uiStateValue testKeyedEvent
type priorityStreamValue testKeyedEvent
type panicAnalyticsValue testKeyedEvent
type healthyAnalyticsValue testKeyedEvent
type obsStreamValue int

func wireAnalyticsBlocker(workspace *Workspace, release chan struct{}) *Feed {
	RegisterSinkClass(
		workspace,
		ServiceAnalytics,
		DeliveryReliableFIFO,
		func(analyticsBlockerValue) string { return "global" },
		func(analyticsBlockerValue) {
			<-release
		},
	)

	feed := workspace.NewFeed()
	feed.Emit(analyticsBlockerValue{Symbol: "block", Seq: 1})

	return feed
}

func WorkspaceObservationalTest(t *testing.T) {
	Convey("Given an observational subscriber whose consumer never advances", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)
		feed := workspace.NewFeed()

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
			RegisterSinkClass(
				workspace,
				ServiceAnalytics,
				DeliveryObservationalFIFO,
				func(obsStreamValue) string { return globalKey },
				func(obsStreamValue) {
					// Never complete: block until the workspace cancels, so the
					// consumer's sequence stays pinned and the ring cannot refill.
					<-workspace.ctx.Done()
				},
			)

			// Publish far more than the ring can hold. Every publication is a
			// TryReserve; the first capacity's worth are accepted and committed,
			// the remainder must fail non-blockingly and be counted.
			const capacity = int(subscriberCapacity)
			const extra = 100

			publishStarted := time.Now()

			for index := 0; index < capacity+extra; index++ {
				feed.Emit(obsStreamValue(index))
			}

			elapsed := time.Since(publishStarted)

			// Non-blocking: publishing 65,636 attempts must finish fast.
			So(elapsed, ShouldBeLessThan, 2*time.Second)

			snap := TypeSnapshot[obsStreamValue](workspace)

			// Exactly one physical ring of exactly the declared capacity.
			So(snap.Dropped, ShouldEqual, uint64(extra))
			So(snap.TryReserveFail, ShouldEqual, uint64(extra))
			So(snap.Published, ShouldEqual, uint64(capacity))
			So(snap.Capacity, ShouldEqual, uint64(capacity))
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

type benchValue int64

func BenchmarkWorkspacePublish(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	workspace := NewWorkspace(ctx)
	feed := workspace.NewFeed()

	defer func() {
		workspace.Close()
		cancel()
	}()

	var counter atomic.Int64
	RegisterSink(workspace, nil, func(benchValue) {
		counter.Add(1)
	})

	b.ReportAllocs()

	for b.Loop() {
		feed.Emit(benchValue(1))
	}

	_ = workspace.WaitForQuiescence(5 * time.Second)
}

func BenchmarkWorkspace640Symbols(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	workspace := NewWorkspace(ctx)
	feed := workspace.NewFeed()

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
	RegisterSink(
		workspace,
		func(event testKeyedEvent) string {
			return event.Symbol
		},
		func(event testKeyedEvent) {
			processedCount.Add(1)
		},
	)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for index := 0; index < symbolCount; index++ {
			feed.Emit(testKeyedEvent{
				Symbol: symbols[index],
				Seq:    index,
			})
		}
	}

	_ = workspace.WaitForQuiescence(10 * time.Second)
}

/*
BenchmarkWorkspaceFanout measures the production signal fan-out shape: N keyed
subscribers all consuming the same ticker type with a no-op Step, so the
broadcast-handler-group cost (every handler scanning every sequence range) is
isolated from signal mathematics. This is the empty-runtime regression boundary:
it must comfortably exceed 1,000 events/sec — by orders of magnitude — before
any signal work is considered.
*/
func BenchmarkWorkspaceFanout(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	workspace := NewWorkspace(ctx)
	feed := workspace.NewFeed()

	defer func() {
		workspace.Close()
		cancel()
	}()

	const symbolCount = 640
	const subscriberCount = 8

	symbols := make([]string, symbolCount)
	for index := 0; index < symbolCount; index++ {
		symbols[index] = fmt.Sprintf("SYM_%d", index)
	}

	var processed atomic.Int64

	for subscriber := 0; subscriber < subscriberCount; subscriber++ {
		RegisterSink(
			workspace,
			func(event testKeyedEvent) string { return event.Symbol },
			func(event testKeyedEvent) {
				processed.Add(1)
			},
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for index := 0; index < symbolCount; index++ {
			feed.Emit(testKeyedEvent{Symbol: symbols[index], Seq: index})
		}
	}

	_ = workspace.WaitForQuiescence(10 * time.Second)
}

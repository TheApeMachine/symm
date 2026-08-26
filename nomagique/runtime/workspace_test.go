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

			// Sink to confirm stage 1 output reached the chained subscriber.
			workspace.Wire("func_in", "", func(_ any) any {
				waitGroup.Done()
				return nil
			})

			workspace.Publish("node_in", 21)

			waitGroup.Wait()
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

		Convey("When one subscriber is slow, fast subscriber continues independently", func() {
			var fastCount atomic.Int64
			var slowCount atomic.Int64
			const eventCount = 100

			workspace.Wire("shared_stream", "", func(input any) any {
				time.Sleep(5 * time.Millisecond)
				slowCount.Add(1)
				return nil
			})

			workspace.Wire("shared_stream", "", func(input any) any {
				fastCount.Add(1)
				return nil
			})

			for index := 0; index < eventCount; index++ {
				workspace.Publish("shared_stream", index)
			}

			time.Sleep(20 * time.Millisecond)
			So(fastCount.Load(), ShouldEqual, int64(eventCount))

			err := workspace.WaitForQuiescence(3 * time.Second)
			So(err, ShouldBeNil)
			So(slowCount.Load(), ShouldEqual, int64(eventCount))
		})
	})
}

func WorkspaceConcurrencyTest(t *testing.T) {
	Convey("Given a Workspace with multi-worker KeyedExecutor", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		workspace := NewWorkspace(ctx)

		Reset(func() {
			workspace.Close()
			cancel()
		})

		Convey("When publishing across 640 unique symbols, different symbols execute concurrently", func() {
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

		Convey("When publishing many events for the same symbol, same symbol never executes concurrently", func() {
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

		Convey("When publishing 10,000 sequential events for one symbol, same-symbol ordering is exact", func() {
			const eventCount = 10000
			expectedSeq := 0
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
					if event.Seq != expectedSeq {
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

			// Ensure SLOW is picked up
			for !slowStarted.Load() {
				time.Sleep(1 * time.Millisecond)
			}

			for index := 0; index < 50; index++ {
				workspace.Publish("multi_symbol_stream", testKeyedEvent{
					Symbol: fmt.Sprintf("FAST-%d", index),
					Seq:    index,
				})
			}

			time.Sleep(20 * time.Millisecond)
			So(fastCount.Load(), ShouldEqual, 50)
			So(slowFinished.Load(), ShouldBeFalse)

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
			// Goroutines should not scale with 10,000 events
			So(currentGoroutines-initialGoroutines, ShouldBeLessThan, 50)
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
	WorkspaceConcurrencyTest(t)
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
	var waitGroup sync.WaitGroup
	workspace.Wire("bench", "", func(_ any) any {
		counter.Add(1)
		waitGroup.Done()
		return nil
	})

	b.ReportAllocs()

	for b.Loop() {
		waitGroup.Add(1)
		workspace.Publish("bench", int64(1))
	}

	waitGroup.Wait()
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
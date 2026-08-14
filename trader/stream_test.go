package trader

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestStreamPipelineDispatch(t *testing.T) {
	Convey("Given one local shard", t, func() {
		wake := make(chan struct{}, 1)
		inbox, err := newLane[marketEvent](4, 0, wake)
		So(err, ShouldBeNil)
		pipeline := &streamPipeline{
			local: []*streamWorker{{
				name:  "local-0",
				inbox: inbox,
			}},
		}
		symbol := types.NewSymbol("AKE/USD", nil)

		Convey("It should keep a later book from joining an already queued remeasure", func() {
			So(pipeline.Dispatch(marketEvent{
				sequence: 1,
				kind:     marketEventBook,
				symbol:   symbol,
			}), ShouldBeNil)
			So(pipeline.Dispatch(marketEvent{
				sequence: 2,
				kind:     marketEventBook,
				symbol:   symbol,
			}), ShouldBeNil)

			So(inbox.telemetry().Depth, ShouldEqual, 1)
			So(pipeline.coalescedBooks.Load(), ShouldEqual, uint64(1))
			So(pipeline.committedSequence.Load(), ShouldEqual, uint64(1))
		})

		Convey("It should keep Level 3 off the ticker inbox", func() {
			level3, err := newLane[marketEvent](4, 0, wake)
			So(err, ShouldBeNil)
			pipeline.local[0].level3 = level3
			So(pipeline.Dispatch(marketEvent{
				sequence: 1,
				kind:     marketEventTicker,
				symbol:   symbol,
			}), ShouldBeNil)
			So(pipeline.Dispatch(marketEvent{
				sequence: 2,
				kind:     marketEventLevel3,
				symbol:   symbol,
			}), ShouldBeNil)

			So(inbox.telemetry().Depth, ShouldEqual, 1)
			So(level3.telemetry().Depth, ShouldEqual, 1)
		})

		Convey("It should number tickers on their own collect sequence", func() {
			So(pipeline.Dispatch(marketEvent{
				sequence: 1,
				kind:     marketEventTicker,
				symbol:   symbol,
			}), ShouldBeNil)

			So(inbox.telemetry().Depth, ShouldEqual, 1)
			So(pipeline.tickerSequence.Load(), ShouldEqual, uint64(1))
		})
	})
}

func TestStreamPipelineWait(t *testing.T) {
	Convey("Given one committed event from a sparse market stream", t, func() {
		pipelineContext, cancelPipeline := context.WithCancel(t.Context())
		pipeline := &streamPipeline{
			ctx:      pipelineContext,
			cancel:   cancelPipeline,
			progress: make(chan struct{}, 1),
		}
		pipeline.committedSequence.Store(7)
		Reset(cancelPipeline)

		Convey("It should wait only for the requested ingress sequence", func() {
			err := pipeline.Wait(t.Context(), 7)

			So(err, ShouldBeNil)
		})

		Convey("It should expose cancellation while a later sequence is pending", func() {
			waitContext, cancelWait := context.WithCancel(t.Context())
			cancelWait()

			err := pipeline.Wait(waitContext, 8)

			So(errors.Is(err, context.Canceled), ShouldBeTrue)
		})
	})
}

func TestStreamPipelineCollect(t *testing.T) {
	Convey("Given completed measurements waiting behind a stopped commit owner", t, func() {
		pipelineContext, cancelPipeline := context.WithCancel(t.Context())
		resultWake := make(chan struct{}, 1)
		commitWake := make(chan struct{}, 1)
		outbox, err := newLane[measurementResult](4, 0, resultWake)
		So(err, ShouldBeNil)
		commitInbox, err := newLane[*eventResults](4, 0, commitWake)
		So(err, ShouldBeNil)
		pipeline := &streamPipeline{
			ctx:         pipelineContext,
			cancel:      cancelPipeline,
			config:      streamConfig{drainLimit: 4},
			workers:     []*streamWorker{{outbox: outbox}},
			resultWake:  resultWake,
			commitInbox: commitInbox,
			commitWake:  commitWake,
		}
		var wait sync.WaitGroup
		wait.Add(1)
		go pipeline.collect(&wait)
		Reset(func() {
			cancelPipeline()
			wait.Wait()
		})

		for sequence := uint64(1); sequence <= 4; sequence++ {
			event := marketEvent{
				sequence:       sequence,
				tickerSequence: sequence,
				parts:          1,
				kind:           marketEventTicker,
			}
			result := measurementResult{
				event: event,
				measurements: []*types.Measurement{{
					Symbol: "BTC/USD",
				}},
			}
			So(outbox.Push(t.Context(), result), ShouldBeNil)
		}

		deadline := time.Now().Add(time.Second)

		for commitInbox.telemetry().Depth < 4 && time.Now().Before(deadline) {
			runtime.Gosched()
		}

		Convey("It should keep draining workers while ordered commit is unavailable", func() {
			So(outbox.telemetry().Depth, ShouldEqual, 0)
			So(commitInbox.telemetry().Depth, ShouldEqual, 4)
		})
	})

	Convey("Given a book result ahead of the first ticker sequence", t, func() {
		pipelineContext, cancelPipeline := context.WithCancel(t.Context())
		resultWake := make(chan struct{}, 1)
		commitWake := make(chan struct{}, 1)
		outbox, err := newLane[measurementResult](4, 0, resultWake)
		So(err, ShouldBeNil)
		commitInbox, err := newLane[*eventResults](4, 0, commitWake)
		So(err, ShouldBeNil)
		pipeline := &streamPipeline{
			ctx:         pipelineContext,
			cancel:      cancelPipeline,
			config:      streamConfig{drainLimit: 4},
			workers:     []*streamWorker{{outbox: outbox}},
			resultWake:  resultWake,
			commitInbox: commitInbox,
			commitWake:  commitWake,
		}
		var wait sync.WaitGroup
		wait.Add(1)
		go pipeline.collect(&wait)
		Reset(func() {
			cancelPipeline()
			wait.Wait()
		})

		So(outbox.Push(t.Context(), measurementResult{
			event: marketEvent{sequence: 40, parts: 1, kind: marketEventBook},
		}), ShouldBeNil)
		So(outbox.Push(t.Context(), measurementResult{
			event: marketEvent{
				sequence:       41,
				tickerSequence: 1,
				parts:          1,
				kind:           marketEventTicker,
			},
		}), ShouldBeNil)

		deadline := time.Now().Add(time.Second)

		for commitInbox.telemetry().Depth < 2 && time.Now().Before(deadline) {
			runtime.Gosched()
		}

		Convey("It should commit the book without waiting for earlier ticker holes", func() {
			So(commitInbox.telemetry().Depth, ShouldEqual, 2)
			So(pipeline.nextSequence.Load(), ShouldEqual, uint64(2))
		})
	})
}

func TestStreamPipelineCommitEvent(t *testing.T) {
	Convey("Given an ordered ticker commit without normalized signal output", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("AKE/USD", nil)
		thesis.Symbols.Store(symbol.Symbol, symbol)
		pipeline := &streamPipeline{
			ctx:               t.Context(),
			thesis:            thesis,
			progress:          make(chan struct{}, 1),
			measurementsDirty: make(map[string]bool),
			resonanceDirty:    make(map[string]bool),
			analyzed:          map[string]int64{symbol.Symbol: 7},
		}
		results := &eventResults{event: marketEvent{
			sequence: 1,
			tick:     7,
			kind:     marketEventTicker,
			symbol:   symbol,
			at:       time.Unix(7, 0),
			ticker: kraken.TickerData{
				Symbol: "AKE/USD",
				Ask:    decimal.NewFromFloat64(101),
				Bid:    decimal.NewFromFloat64(99),
			},
		}}

		So(pipeline.commitEvent(results), ShouldBeNil)

		Convey("It should enqueue the midpoint and wake the predictor", func() {
			rows := make([]*types.ResonanceMeasurement, 0)

			for row := range symbol.ResonanceMeasurements() {
				rows = append(rows, row)
			}

			So(rows, ShouldHaveLength, 1)
			So(rows[0].Tick, ShouldEqual, int64(7))
			So(rows[0].Mark, ShouldEqual, 100.0)
			So(pipeline.measurementsDirty[symbol.Symbol], ShouldBeFalse)
			So(pipeline.resonanceDirty[symbol.Symbol], ShouldBeTrue)
			So(pipeline.committedSequence.Load(), ShouldEqual, uint64(1))
		})
	})
}

func BenchmarkStreamPipelineDrainResults(b *testing.B) {
	resultWake := make(chan struct{}, 1)
	outbox, err := newLane[measurementResult](1024, 0, resultWake)

	if err != nil {
		b.Fatal(err)
	}

	pipeline := &streamPipeline{
		config:  streamConfig{drainLimit: 1024},
		workers: []*streamWorker{{outbox: outbox}},
	}
	pending := make(map[uint64]*eventResults, 1024)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()

		for sequence := uint64(1); sequence <= 1024; sequence++ {
			if err := outbox.Push(b.Context(), measurementResult{
				event: marketEvent{
					sequence:       sequence,
					tickerSequence: sequence,
					parts:          1,
					kind:           marketEventTicker,
				},
			}); err != nil {
				b.Fatal(err)
			}
		}

		clear(pending)
		b.StartTimer()
		worked, err := pipeline.drainResults(pending)

		if err != nil {
			b.Fatal(err)
		}

		if !worked || len(pending) != 1024 {
			b.Fatalf("collected %d completed events", len(pending))
		}
	}
}

func BenchmarkStreamPipelineWait(b *testing.B) {
	pipeline := &streamPipeline{
		ctx:      b.Context(),
		progress: make(chan struct{}, 1),
	}
	pipeline.committedSequence.Store(1)
	b.ReportAllocs()

	for b.Loop() {
		if err := pipeline.Wait(b.Context(), 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStreamPipelineCommitEvent(b *testing.B) {
	thesis := types.NewThesis(b.Context(), nil)
	symbol := types.NewSymbol("AKE/USD", nil)
	thesis.Symbols.Store(symbol.Symbol, symbol)
	pipeline := &streamPipeline{
		ctx:               b.Context(),
		thesis:            thesis,
		progress:          make(chan struct{}, 1),
		measurementsDirty: make(map[string]bool),
		resonanceDirty:    make(map[string]bool),
		analyzed:          make(map[string]int64),
	}
	ask := decimal.NewFromFloat64(101)
	bid := decimal.NewFromFloat64(99)
	b.ReportAllocs()

	for index := range b.N {
		tick := int64(index + 1)
		pipeline.analyzed[symbol.Symbol] = tick
		results := &eventResults{event: marketEvent{
			sequence: uint64(index + 1),
			tick:     tick,
			kind:     marketEventTicker,
			symbol:   symbol,
			at:       time.Unix(tick, 0),
			ticker: kraken.TickerData{
				Symbol: symbol.Symbol,
				Ask:    ask,
				Bid:    bid,
			},
		}}

		if err := pipeline.commitEvent(results); err != nil {
			b.Fatal(err)
		}

		for range symbol.ResonanceMeasurements() {
		}
	}
}

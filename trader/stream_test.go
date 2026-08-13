package trader

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStreamPipelineWait(t *testing.T) {
	Convey("Given one committed event from a sparse market stream", t, func() {
		pipelineContext, cancelPipeline := context.WithCancel(t.Context())
		pipeline := &streamPipeline{
			ctx:      pipelineContext,
			cancel:   cancelPipeline,
			progress: make(chan struct{}, 1),
		}
		pipeline.committedSequence = 7
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
			event := marketEvent{sequence: sequence, parts: 1}
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
				event: marketEvent{sequence: sequence, parts: 1},
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

package trader

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamPipelineDiagnostics(t *testing.T) {
	Convey("Given a pipeline whose ingress is still aligned with commit", t, func() {
		pipeline := diagnosticPipeline(t)

		Convey("It should report a flowing empty data plane", func() {
			snapshot := pipeline.Diagnostics()

			So(snapshot.Status, ShouldEqual, "flowing")
			So(snapshot.Lag, ShouldEqual, uint64(0))
			So(snapshot.Lanes, ShouldHaveLength, 2)
			So(snapshot.Lanes[0].Name, ShouldEqual, "local-0.inbox")
			So(snapshot.Lanes[0].Blocking, ShouldBeFalse)
			So(snapshot.Lanes[1].Name, ShouldEqual, "local-0.outbox")
			So(len(snapshot.Stages), ShouldBeGreaterThan, 10)
			So(snapshot.Stages[0].Name, ShouldEqual, "price")
			So(snapshot.Stages[2].Name, ShouldEqual, "crypto")
			So(snapshot.Hops[0].From, ShouldEqual, "price")
			So(snapshot.Hops[0].To, ShouldEqual, "crypto")
		})
	})

	Convey("Given a full blocking cross inbox", t, func() {
		pipeline := diagnosticPipeline(t)
		pipeline.workers[0].name = "correlation"
		pipeline.workers[0].local = false

		for index := range 4 {
			So(pipeline.workers[0].inbox.Push(t.Context(), marketEvent{
				sequence: uint64(index + 1),
			}), ShouldBeNil)
		}

		Convey("It should name the lane that is parking its producer", func() {
			snapshot := pipeline.Diagnostics()

			So(snapshot.Status, ShouldEqual, "stalled")
			So(snapshot.Summary, ShouldContainSubstring, "correlation.inbox")
			So(snapshot.Lanes[0].Depth, ShouldEqual, 4)
			So(snapshot.Lanes[0].Blocking, ShouldBeTrue)
		})
	})

	Convey("Given commit lag older than the diagnostic interval", t, func() {
		pipeline := diagnosticPipeline(t)
		pipeline.ingressSequence.Store(8)
		pipeline.committedSequence.Store(3)
		pipeline.nextSequence.Store(4)
		pipeline.pendingCount.Store(2)
		pipeline.lastCommitNanos.Store(time.Now().Add(-2 * time.Second).UnixNano())

		Convey("It should report the incomplete sequence as a stall", func() {
			snapshot := pipeline.Diagnostics()

			So(snapshot.Status, ShouldEqual, "stalled")
			So(snapshot.Lag, ShouldEqual, uint64(5))
			So(snapshot.Pending, ShouldEqual, int64(2))
			So(snapshot.StallNs, ShouldBeGreaterThan, uint64(time.Second))
			So(snapshot.Summary, ShouldContainSubstring, "sequence 4")
		})
	})

	Convey("Given a short commit lag still inside the diagnostic interval", t, func() {
		pipeline := diagnosticPipeline(t)
		pipeline.ingressSequence.Store(3)
		pipeline.committedSequence.Store(2)
		So(pipeline.workers[0].inbox.Push(t.Context(), marketEvent{
			sequence: 3,
		}), ShouldBeNil)

		Convey("It should keep the headline flowing while work is in flight", func() {
			snapshot := pipeline.Diagnostics()

			So(snapshot.Status, ShouldEqual, "flowing")
			So(snapshot.Lag, ShouldEqual, uint64(1))
			So(snapshot.Lanes[0].Depth, ShouldEqual, 1)
		})
	})

	Convey("Given a local drop after ingress and commit realigned", t, func() {
		pipeline := diagnosticPipeline(t)
		pipeline.dropped.Store(3)
		pipeline.commitDropped.Store(1)

		Convey("It should keep the loss on the counters without flipping the headline", func() {
			snapshot := pipeline.Diagnostics()

			So(snapshot.Status, ShouldEqual, "flowing")
			So(snapshot.Lossy, ShouldBeTrue)
			So(snapshot.Dropped, ShouldEqual, uint64(3))
			So(snapshot.CommitDropped, ShouldEqual, uint64(1))
		})
	})
}

func TestStreamPipelineNoteBroker(t *testing.T) {
	Convey("Given a mark update that has already finished", t, func() {
		pipeline := diagnosticPipeline(t)

		Convey("It should accumulate broker time and arm the trader hop", func() {
			pipeline.noteBroker(time.Now().Add(-time.Millisecond))
			snapshot := pipeline.Diagnostics()
			price := snapshot.Stages[0]

			So(price.Name, ShouldEqual, "price")
			So(price.Count, ShouldEqual, uint64(1))
			So(price.LastNs, ShouldBeGreaterThan, uint64(0))
			So(price.LastAtNs, ShouldBeGreaterThan, int64(0))
			So(pipeline.lastBrokerAt.IsZero(), ShouldBeFalse)
		})
	})
}

func TestStreamPipelineStampCollected(t *testing.T) {
	Convey("Given a measured event waiting in an outbox", t, func() {
		pipeline := diagnosticPipeline(t)
		event := marketEvent{measuredAt: time.Now().Add(-time.Millisecond)}

		Convey("It should stamp collect time onto the hop from signals", func() {
			pipeline.stampCollected(&event)
			snapshot := pipeline.Diagnostics()
			hop := hopNamed(snapshot.Hops, "signals", "collect")

			So(event.collectedAt.IsZero(), ShouldBeFalse)
			So(hop.Count, ShouldEqual, uint64(1))
			So(hop.LastNs, ShouldBeGreaterThan, uint64(0))
		})
	})
}

func TestStreamPipelinePublishDiagnostics(t *testing.T) {
	Convey("Given a pipeline that emits replaceable diagnostics", t, func() {
		ui := make(chan []byte, 1)
		ctx, cancel := context.WithCancel(t.Context())
		pipeline := diagnosticPipeline(t)
		pipeline.ctx = ctx
		pipeline.cancel = cancel
		pipeline.ui = ui
		pipeline.config.diagnosticInterval = 10 * time.Millisecond
		var wait sync.WaitGroup
		wait.Add(1)
		go pipeline.publishDiagnostics(&wait)
		Reset(func() {
			cancel()
			wait.Wait()
		})

		Convey("It should publish a diagnostics frame on the heartbeat", func() {
			var payload []byte

			select {
			case payload = <-ui:
			case <-time.After(time.Second):
			}

			So(payload, ShouldNotBeNil)
			So(string(payload), ShouldContainSubstring, "diagnostics")
			So(string(payload), ShouldContainSubstring, "flowing")
		})
	})
}

func diagnosticPipeline(testingTB testing.TB) *streamPipeline {
	testingTB.Helper()
	wake := make(chan struct{}, 1)
	inbox, err := newLane[marketEvent](4, 0, wake)

	if err != nil {
		testingTB.Fatalf("inbox: %v", err)
	}

	outbox, err := newLane[measurementResult](4, 0, wake)

	if err != nil {
		testingTB.Fatalf("outbox: %v", err)
	}

	pipeline := &streamPipeline{
		config: streamConfig{diagnosticInterval: time.Second},
		workers: []*streamWorker{{
			name:   "local-0",
			local:  true,
			inbox:  inbox,
			outbox: outbox,
		}},
	}
	pipeline.nextSequence.Store(1)
	pipeline.lastCommitNanos.Store(time.Now().UnixNano())

	return pipeline
}

func hopNamed(hops []HopSnapshot, from string, to string) HopSnapshot {
	for _, hop := range hops {
		if hop.From == from && hop.To == to {
			return hop
		}
	}

	return HopSnapshot{}
}

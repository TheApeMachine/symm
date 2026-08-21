package trader

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

func TestBindDiagnostics(t *testing.T) {
	Convey("Given registered signal, resonance, and desk workers", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		thesis := types.NewThesis(ctx, nil)
		diagnostics := &Diagnostics{
			started:  time.Now(),
			interval: time.Hour,
		}
		crypto := &Crypto{
			ctx:         ctx,
			thesis:      thesis,
			diagnostics: diagnostics,
			manifold:    transport.NewMapReduce[types.FluidFrame](nil, nil, nil),
		}
		crypto.bindDiagnostics()
		sources := []types.SourceType{
			types.SourceCorrelation,
			types.SourceResonance,
			types.SourceDesk,
		}

		for _, source := range sources {
			consumer := transport.NewConsumer[*types.Symbol](string(source), func() {})
			work := thesis.Work(source)
			work.Register(consumer)
			work.Push(thesis.Symbol("BTC/USD"))

			for range work.Drain(consumer, nil) {
			}
		}

		Convey("Each completed drain should update its matching stage clock", func() {
			for _, source := range sources {
				clock := diagnostics.module(string(source))
				So(clock.count.Load(), ShouldEqual, 1)
				So(clock.lastAtNs.Load(), ShouldNotEqual, 0)
			}
		})
	})
}

func TestDiagnosticsObserve(t *testing.T) {
	Convey("Score computations roll up into stage clocks", t, func() {
		diagnostics := &Diagnostics{}

		Convey("A module accumulates count, total, last, and max", func() {
			diagnostics.applyModule("category", 100*time.Millisecond)
			diagnostics.applyModule("category", 300*time.Millisecond)

			clock := diagnostics.module("category")
			So(clock.count.Load(), ShouldEqual, 2)
			So(clock.totalNs.Load(), ShouldEqual, 400_000_000)
			So(clock.lastNs.Load(), ShouldEqual, 300_000_000)
			So(clock.maxNs.Load(), ShouldEqual, 300_000_000)
			So(clock.lastAtNs.Load(), ShouldBeGreaterThan, 0)
		})

		Convey("An empty name is ignored", func() {
			diagnostics.applyModule("", 10*time.Millisecond)
			So(diagnostics.module("").count.Load(), ShouldEqual, 0)
		})

		Convey("An in-flight operation remains distinct from completed work", func() {
			diagnostics.beginModule("category")
			started := diagnostics.module("category").snapshot("category")

			So(started.Active, ShouldEqual, 1)
			So(started.Count, ShouldEqual, 0)
			So(started.StartedNs, ShouldNotEqual, 0)

			diagnostics.completeModule("category", 25*time.Millisecond)
			completed := diagnostics.module("category").snapshot("category")

			So(completed.Active, ShouldEqual, 0)
			So(completed.Count, ShouldEqual, 1)
			So(completed.LastNs, ShouldEqual, 25_000_000)
		})
	})

	Convey("Hops record the wait between two systems", t, func() {
		diagnostics := &Diagnostics{}

		diagnostics.applyHop("category", "causal", 5*time.Millisecond)
		diagnostics.applyHop("category", "causal", 15*time.Millisecond)

		clock := diagnostics.clocks.hop("category", "causal")
		So(clock.count.Load(), ShouldEqual, 2)
		So(clock.totalNs.Load(), ShouldEqual, 20_000_000)
		So(clock.lastNs.Load(), ShouldEqual, 15_000_000)
	})

	Convey("Errors are retained newest-first as subsystem hints", t, func() {
		diagnostics := &Diagnostics{}

		diagnostics.ObserveError("planner", "search failed", "strategy/planner.go:120")
		diagnostics.ObserveError("desk", "order rejected", "broker/desk.go:400")

		errors := diagnostics.errorSnapshots()
		So(errors, ShouldHaveLength, 2)
		So(errors[0].Source, ShouldEqual, "desk")
		So(errors[1].Source, ShouldEqual, "planner")
		So(errors[0].Caller, ShouldContainSubstring, "broker/desk.go")
		So(errors[0].AtNs, ShouldBeGreaterThan, 0)

		Convey("Unattributed errors default to the system source", func() {
			diagnostics.ObserveError("", "boom", "x.go:1")
			So(diagnostics.errorSnapshots()[0].Source, ShouldEqual, "system")
		})
	})

	Convey("Snapshots enumerate every wired stage in order", t, func() {
		diagnostics := &Diagnostics{}
		diagnostics.applyModule("crypto", time.Millisecond)
		diagnostics.applyModule("desk", 2*time.Millisecond)

		stages := diagnostics.stageSnapshots()
		So(stages, ShouldHaveLength, len(stageNames()))
		So(stages[0].Name, ShouldEqual, "crypto")
		So(stages[len(stages)-1].Name, ShouldEqual, "desk")
		So(stages[0].Count, ShouldEqual, 1)
	})

	Convey("A Crypto without diagnostics reports an idle flow", t, func() {
		crypto := &Crypto{}

		frame := crypto.Diagnostics()
		So(frame.Status, ShouldEqual, "idle")
		So(frame.Stages, ShouldHaveLength, 0)

		Convey("A nil crypto is safe", func() {
			var nilCrypto *Crypto
			So(nilCrypto.Diagnostics().Status, ShouldEqual, "idle")
			So(nilCrypto.Diagnostics().Stages, ShouldHaveLength, 0)
		})
	})

	Convey("The measurement pass distinguishes idle from blocked", t, func() {
		diagnostics := &Diagnostics{}
		now := time.Now()

		Convey("An engine that never ran a pass reports gated idle", func() {
			status := diagnostics.passStatus(now)
			So(status.State, ShouldEqual, "idle")
			So(status.LastPassNs, ShouldEqual, 0)
		})

		Convey("A completed pass reports idle with the pass duration", func() {
			diagnostics.ObservePassStart(now.Add(-10 * time.Millisecond))
			diagnostics.ObservePassEnd(now, 10*time.Millisecond)

			status := diagnostics.passStatus(now)
			So(status.State, ShouldEqual, "idle")
			So(status.LastPassNs, ShouldEqual, 10_000_000)
			So(status.SinceLastNs, ShouldEqual, 0)
		})

		Convey("A pass in flight but under the threshold reports running", func() {
			diagnostics.ObservePassStart(now.Add(-500 * time.Millisecond))

			status := diagnostics.passStatus(now)
			So(status.State, ShouldEqual, "running")
			So(status.InFlightNs, ShouldEqual, 500_000_000)
		})

		Convey("A pass over the threshold reports blocked", func() {
			start := time.Now().Add(-blockedPassThreshold - time.Second)
			diagnostics.ObservePassStart(start)

			status := diagnostics.passStatus(time.Now())
			So(status.State, ShouldEqual, "blocked")
			So(status.InFlightNs, ShouldBeGreaterThan, int64(blockedPassThreshold))
		})
	})

	Convey("Queue snapshots aggregate per-symbol buffers across the universe", t, func() {
		thesis := &types.Thesis{
			Symbols: &sync.Map{},
		}
		tickerA := types.NewSymbol("AAA")
		tickerB := types.NewSymbol("BBB")

		thesis.Symbols.Store("AAA", tickerA)
		thesis.Symbols.Store("BBB", tickerB)

		diagnostics := &Diagnostics{
			started: time.Now(),
		}

		crypto := &Crypto{
			thesis:      thesis,
			diagnostics: diagnostics,
			desk:        nil,
		}

		Convey("Reports zero depths when no items have been written", func() {
			queues := crypto.queueSnapshots()
			So(queues, ShouldNotBeEmpty)
			So(len(queues), ShouldEqual, 15)

			Convey("Lists every pipeline wire static writer/reader annotation", func() {
				found := false
				for _, q := range queues {
					if q.Name == "ingress.tickers" {
						found = true
						So(q.Kind, ShouldEqual, "ingress")
						So(q.Readers, ShouldContain, "correlation")
						So(q.Readers, ShouldContain, "pumpdump")
					}

					if q.Name == "measurements" {
						So(q.Kind, ShouldEqual, "rail")
						So(q.Readers, ShouldContain, "category")
						So(q.Readers, ShouldContain, "graph")
					}
				}
				So(found, ShouldBeTrue)
			})
		})

		Convey("Sums depths across every live symbol", func() {
			tickerA.AppendTicker(kraken.TickerData{})
			tickerB.AppendTicker(kraken.TickerData{})

			queues := crypto.queueSnapshots()

			var tickersQueue QueueSnapshot
			for _, q := range queues {
				if q.Name == "ingress.tickers" {
					tickersQueue = q
				}
			}

			So(tickersQueue.Depth, ShouldEqual,
				uint64(2*len(tickerA.TickerConsumers)))
			So(tickersQueue.Symbols, ShouldEqual, 2)
		})
	})
}

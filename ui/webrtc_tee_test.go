package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/signal/hawkes"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestWebRTCTeePush(t *testing.T) {
	Convey("Concurrent consumer publications cross a bounded lock-free queue", t, func() {
		originalRoute := types.Route()
		types.SetRoute("fluid")
		defer types.SetRoute(originalRoute)
		state := &types.ManifoldState{Version: 1}
		tee := NewWebRTCTee(t.Context(), "webrtc-test", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		measurement := &data.Measurement[float64]{Source: "manifold", Result: state}
		tee.Push(measurement)
		So(tee.Next() == nil, ShouldBeTrue)
		tee.Transition(runtime.READY)

		Convey("A full queue preserves accepted entries and resumes after draining", func() {
			for version := uint64(1); version <= 5; version++ {
				measurement.Result = &types.ManifoldState{Version: version}
				tee.Push(measurement)
			}

			for version := uint64(1); version <= 4; version++ {
				So((*(*any)(tee.Next())).(*types.ManifoldState).Version, ShouldEqual, version)
			}

			So(tee.Next() == nil, ShouldBeTrue)
			tee.Push(measurement)
			So((*(*any)(tee.Next())).(*types.ManifoldState).Version, ShouldEqual, 5)
		})

		Convey("Multiple symbols preserve publication order", func() {
			types.SetRoute("xray")
			for _, symbol := range []string{"BTC/USD", "ETH/USD", "BTC/USD"} {
				tee.Push(&data.Measurement[float64]{Source: "resonance", Label: symbol, Result: &types.ResonanceArtifact{Symbol: symbol}})
			}

			seen := map[string]bool{}

			for pointer := tee.Next(); pointer != nil; pointer = tee.Next() {
				artifact := (*(*any)(pointer)).(*types.ResonanceArtifact)
				seen[artifact.Symbol] = true
			}

			So(seen, ShouldResemble, map[string]bool{"BTC/USD": true, "ETH/USD": true})
		})

		Convey("Disallowed results are dropped at Push, not retained for a later route", func() {
			types.SetRoute("journal")
			tee.Push(measurement)
			types.SetRoute("fluid")
			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("A route change discards previously accepted results", func() {
			tee.Push(measurement)
			types.SetRoute("dashboard")
			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("A focus change discards queued dashboard results for the old symbol", func() {
			originalFocus := types.Focus()
			defer types.SetFocus(originalFocus)
			types.SetRoute("dashboard")
			types.SetFocus("BTC/USD")
			tee.Push(&data.Measurement[float64]{Source: "resonance", Label: "BTC/USD", Result: &types.ResonanceArtifact{Symbol: "BTC/USD"}})
			types.SetFocus("ETH/USD")
			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("Concurrent consumers retain their own publication order", func() {
			const producers, observations = 8, 32
			concurrent := NewWebRTCTee(t.Context(), "concurrent", producers*observations)
			defer func() { So(concurrent.Close(), ShouldBeNil) }()
			concurrent.Transition(runtime.READY)
			var workers sync.WaitGroup

			for producer := 0; producer < producers; producer++ {
				workers.Go(func() {
					for sequence := 0; sequence < observations; sequence++ {
						concurrent.Push(&data.Measurement[float64]{Source: "manifold", Result: &types.ManifoldState{
							Version: uint64(producer*observations + sequence),
						}})
					}
				})
			}

			workers.Wait()
			sequences := make([]int, producers)

			for received := 0; received < producers*observations; received++ {
				pointer := concurrent.Next()
				So(pointer == nil, ShouldBeFalse)
				version := int((*(*any)(pointer)).(*types.ManifoldState).Version)
				producer := version / observations
				So(version%observations, ShouldEqual, sequences[producer])
				sequences[producer]++
			}

			So(concurrent.Next() == nil, ShouldBeTrue)
		})

		Convey("Unrelated measurements never enter WebRTC", func() {
			tee.Push(&data.Measurement[float64]{Source: "public", Label: "BTC/USD"})
			So(tee.Next() == nil, ShouldBeTrue)
		})
	})
}

func BenchmarkWebRTCTeePush(b *testing.B) {
	originalRoute := types.Route()
	types.SetRoute("fluid")
	defer types.SetRoute(originalRoute)
	state := &types.ManifoldState{Version: 1}
	tee := NewWebRTCTee(b.Context(), "benchmark", 131072)
	tee.Transition(runtime.READY)
	measurement := &data.Measurement[float64]{Source: "manifold", Result: state}

	for b.Loop() {
		tee.Push(measurement)

		if *(*any)(tee.Next()) != state {
			b.Fatal("published state changed")
		}
	}
}

// publicationSource supplies successive venue observations through the real consumer.
type publicationSource struct{ current *data.Measurement[float64] }

func (source *publicationSource) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("public", nil)
}
func (source *publicationSource) Step(_ *data.Measurement[float64]) *data.Measurement[float64] {
	return source.current
}

func TestWebRTCTeeNext(t *testing.T) {
	Convey("Consumers tee the actual Hawkes and resonance outputs for each arriving symbol", t, func() {
		originalRoute := types.Route()
		types.SetRoute("xray")
		defer types.SetRoute(originalRoute)
		register := store.NewRegister[*data.Measurement[float64]]()
		source := &publicationSource{}
		sourceConsumer := runtime.NewConsumer(source, register).SetPeerLimit(0)
		signal := hawkes.NewTrade(t.Context())
		signal.Transition(runtime.READY)
		defer func() { So(signal.Close(), ShouldBeNil) }()
		signalConsumer := runtime.NewConsumer(signal, register).SetPeerLimit(sourceConsumer.Identity() + 1)
		solver := resonance.NewSolver(t.Context(), 0.05)
		solver.Transition(runtime.READY)
		defer func() { So(solver.Close(), ShouldBeNil) }()
		tee := NewWebRTCTee(t.Context(), "registered-results", 131072)
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		solverConsumer := runtime.NewConsumer(solver, register, tee).SetPeerLimit(signalConsumer.Identity() + 1)
		counts := map[string]int{}
		for index, symbol := range []string{"BTC/USD", "ETH/USD", "BTC/USD", "ETH/USD"} {
			observation := source.Register()
			observation.Label = symbol
			observation.At = time.Unix(100+int64(index), 0)
			observation.Provenance["side"] = "buy"
			observation.Provenance["channel"] = "trade"
			observation.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: 100 + float64(index)}
			observation.Metrics["qty"] = data.Metric[float64]{Label: "qty", Raw: 1}
			source.current = observation
			sequence := int64(index)
			sourceConsumer.Handle(sequence, sequence)
			signalConsumer.Handle(sequence, sequence)
			measured := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*store.NewQuery(signalConsumer, data.ActionRead))))
			counts[symbol]++
			So(measured.Err, ShouldBeNil)
			So(measured.Label, ShouldEqual, symbol)
			So(measured.Metrics["event_count"].Raw, ShouldEqual, counts[symbol])
			solverConsumer.Handle(sequence, sequence)

			pointer := tee.Next()
			So(pointer == nil, ShouldBeFalse)
			result := (*(*any)(pointer)).(*types.ResonanceArtifact)
			So(result.Symbol, ShouldEqual, symbol)
			So(result.At, ShouldEqual, observation.At)
			So(result.Snapshot, ShouldNotBeNil)
			So(len(result.Snapshot.Latent), ShouldBeGreaterThan, 0)
		}
	})
}

func TestNewWebRTCTee(t *testing.T) {
	Convey("The Tee inherits the application context and starts idle", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		tee := NewWebRTCTee(ctx, "context-test", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		So(tee.Status(), ShouldEqual, runtime.INIT)
		cancel()
		So(tee.Context().Err(), ShouldEqual, context.Canceled)
	})
}

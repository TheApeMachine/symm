package ui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
eventually polls a condition with a deadline so the test does not rely on a
single fixed sleep and cannot pass spuriously on a slow CI machine.
*/
func eventually(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("condition never became true within deadline")
}

func TestDiagnosticsPublishing(t *testing.T) {
	Convey("Given a diagnostics collector on a live workspace", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(ctx)
		defer bus.Close()

		collector := NewDiagnostics(ctx, bus)
		defer collector.Close()

		var receivedUI atomic.Int64
		var receivedFluid atomic.Int64

		runtime.RegisterSink(bus, nil, func(frame *types.UIFrame) {
			if frame != nil && frame.Type == wire.FrameDiagnosticsFrame {
				receivedUI.Add(1)
			}
		})

		runtime.RegisterSink(bus, nil, func(payload []byte) {
			if len(payload) > 0 {
				receivedFluid.Add(1)
			}
		})

		collector.publish()

		Convey("Both the dashboard UI stream and the fluid WebRTC channel should receive a diagnostics frame", func() {
			eventually(t, func() bool {
				return receivedUI.Load() > 0 && receivedFluid.Load() > 0
			})

			So(receivedUI.Load(), ShouldBeGreaterThan, 0)
			So(receivedFluid.Load(), ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestDiagnosticsEndToEndDelivery is an adversarial exact-input/exact-output test.
It drives the real production publish path, decodes the FlatBuffer payload a
browser would receive, and asserts exact names, kinds, wiring, ring capacity,
and stage timing rather than "non-empty" or "greater than zero". If the
forwarder, the encode step, the fan-out, or any queue entry is altered or
removed, this fails.
*/
func TestDiagnosticsEndToEndDelivery(t *testing.T) {
	Convey("Given a fully wired diagnostics pipeline", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(ctx)
		defer bus.Close()

		collector := NewDiagnostics(ctx, bus)
		defer collector.Close()

		Convey("A heartbeat must reconstruct the exact 15-queue topology, byte-for-byte", func() {
			var received atomic.Value

			runtime.RegisterSink(bus, nil, func(payload []byte) {
				if len(payload) > 0 {
					received.Store(payload)
				}
			})

			collector.publish()

			eventually(t, func() bool { return received.Load() != nil })

			payload := received.Load().([]byte)

			envelope, err := telemetry.Decode(payload)
			So(err, ShouldBeNil)

			diag, ok := envelope.Frame.Value.(*wire.DiagnosticsFrameT)
			So(ok, ShouldBeTrue)
			So(diag.Status, ShouldEqual, "flowing")
			So(diag.Enabled, ShouldBeTrue)

			// The exact topology, in the exact emission order.
			So(len(diag.Queues), ShouldEqual, 16)

			type want struct {
				name            string
				kind            string
				writers, readers []string
			}

			wants := []want{
				{"ingress.tickers", "ingress",
					[]string{"crypto"},
					[]string{"correlation", "leadlag", "liquidity", "pumpdump", "sentiment", "exhaustion", "resonance", "desk"}},
				{"ingress.trades", "ingress",
					[]string{"crypto"},
					[]string{"cvd", "derivatives", "exhaustion", "hawkes", "pumpdump", "toxicity"}},
				{"ingress.level3", "ingress", []string{"crypto"}, []string{"depthflow"}},
				{"measurements", "rail",
					[]string{"correlation", "cvd", "depthflow", "derivatives", "exhaustion", "hawkes", "leadlag", "liquidity", "pumpdump", "sentiment", "toxicity"},
					[]string{"category", "manifold", "graph"}},
				{"derived.category", "derived", []string{"category"}, []string{"cognition", "graph"}},
				{"derived.causal", "derived", []string{"causal"}, []string{"graph"}},
				{"derived.cognition", "derived", []string{"cognition"}, []string{"graph"}},
				{"derived.graph", "derived", []string{"graph"}, []string{"planner"}},
				{"derived.resonance", "derived", []string{"resonance"}, []string{"causal", "graph"}},
				{"decisions", "strategy", []string{"planner"}, []string{"mcts", "allocation", "desk"}},
				{"desk.ticker", "broker", []string{"crypto"}, []string{"desk"}},
				{"desk.executions", "broker", []string{"websocket-api"}, []string{"desk"}},
				{"positions", "broker", []string{"desk"}, []string{"audit", "hub"}},
				{"ui.dashboard", "ui",
					[]string{"crypto", "category", "manifold", "causal", "cognition", "graph", "resonance", "planner", "allocation", "desk", "diagnostics"},
					[]string{"hub"}},
				{"ui.manifold", "ui", []string{"manifold"}, []string{"webrtc-hub"}},
				{"ui.diagnostics", "ui", []string{"diagnostics"}, []string{"webrtc-hub"}},
			}

			for index, expected := range wants {
				queue := diag.Queues[index]

				So(queue.Name, ShouldEqual, expected.name)
				So(queue.Kind, ShouldEqual, expected.kind)
				So(queue.Writers, ShouldResemble, expected.writers)
				So(queue.Readers, ShouldResemble, expected.readers)

				// Ring capacity is a stable infrastructure constant.
				So(queue.Capacity, ShouldEqual, runtime.SubscriberCapacity)

				// An empty ring reports exact zero backlog.
				So(queue.Depth, ShouldEqual, 0)
				So(queue.HighWater, ShouldEqual, 0)

				// Exactly one subscriber is registered by this test (the capture
				// wire on topic "diagnostics"), and unkeyed wires run a single
				// handler lane, so only the ui.diagnostics queue — which reads
				// topic "diagnostics" — reports that lane; every other queue has
				// zero registered lanes.
				expectedLanes := uint64(0)
				if expected.name == "ui.diagnostics" {
					expectedLanes = 1
				}

				So(queue.Symbols, ShouldEqual, expectedLanes)
			}
		})

		Convey("ObserveModule must surface a stage with the exact summed nanoseconds", func() {
			collector.applyModule("correlation", 150*time.Microsecond)
			collector.applyModule("correlation", 250*time.Microsecond)

			snapshot := collector.Snapshot()

			found := false

			for _, stage := range snapshot.Stages {
				if stage.Name != "correlation" {
					continue
				}

				found = true
				So(stage.Count, ShouldEqual, 2)
				So(stage.TotalNs, ShouldEqual, 400000)
				So(stage.LastNs, ShouldEqual, 250000)
				So(stage.MaxNs, ShouldEqual, 250000)
			}

			So(found, ShouldBeTrue)
		})

		Convey("Disabling collection must produce the exact disabled frame", func() {
			collector.SetDiagnosticsEnabled(false)

			frame := collector.Snapshot()
			So(frame.Enabled, ShouldBeFalse)
			So(frame.Status, ShouldEqual, "disabled")
			So(len(frame.Stages), ShouldEqual, 0)
			So(len(frame.Queues), ShouldEqual, 0)

			collector.SetDiagnosticsEnabled(true)
			So(collector.DiagnosticsEnabled(), ShouldBeTrue)
		})
	})
}

func BenchmarkDiagnosticsWire(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := runtime.NewWorkspace(ctx)
	defer bus.Close()

	collector := NewDiagnostics(ctx, bus)
	defer collector.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = collector.Snapshot().Wire()
	}
}

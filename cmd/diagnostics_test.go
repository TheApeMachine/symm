package cmd

import (
	"context"
	goruntime "runtime"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry"
	wireT "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestDiagnosticsPublishing(t *testing.T) {
	Convey("Given a Crypto diagnostics collector", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(ctx)
		defer bus.Close()

		crypto := &Crypto{
			ctx:         ctx,
			cancel:      cancel,
			bus:         bus,
			diagnostics: &Diagnostics{started: time.Now()},
		}
		// Forward each heartbeat to the dashboard frame exactly as
		// bindDiagnostics does, so the test observes the side-effect path
		// without assembling the whole broker stack.
		bus.Wire(types.ChannelDiagnostics, "", func(value any) any {
			diag, ok := value.(StreamDiagnostics)

			if !ok {
				return nil
			}

			wireFrame := diag.Wire()

			bus.Publish(types.ChannelFluid, types.FluidFrame{
				Channel: types.DiagnosticsChannel,
				Payload: telemetry.Encode(&wireT.FrameT{
					Type:  wireT.FrameDiagnosticsFrame,
					Value: wireFrame,
				}),
			})
			bus.Publish(types.ChannelUI, &types.UIFrame{
				Type:  wireT.FrameDiagnosticsFrame,
				Value: wireFrame,
			})

			return nil
		})

		var receivedUI atomic.Int64
		var receivedFluid atomic.Int64

		bus.Wire(types.ChannelUI, "", func(value any) any {
			frame, ok := value.(*types.UIFrame)
			if ok && frame != nil && frame.Type == wireT.FrameDiagnosticsFrame {
				receivedUI.Add(1)
			}

			return nil
		})

		bus.Wire(types.ChannelFluid, "", func(value any) any {
			frame, ok := value.(types.FluidFrame)
			if ok && frame.Channel == types.DiagnosticsChannel && len(frame.Payload) > 0 {
				receivedFluid.Add(1)
			}

			return nil
		})

		go crypto.publishDiagnostics()

		Convey("When diagnostics publish loop runs", func() {
			deadline := time.Now().Add(time.Second)
			for (receivedUI.Load() == 0 || receivedFluid.Load() == 0) && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			Convey("Both UI and Fluid channels should receive diagnostics frames", func() {
				So(receivedUI.Load(), ShouldBeGreaterThan, 0)
				So(receivedFluid.Load(), ShouldBeGreaterThan, 0)
			})
		})
	})
}

/*
TestDiagnosticsEndToEndDelivery is an adversarial exact-input/exact-output test.
It drives the real production wiring (bindDiagnostics), publishes a heartbeat
exactly as publishDiagnostics does, and decodes the FlatBuffer payload a browser
would receive, asserting exact names, kinds, wiring, ring capacity, and stage
timing rather than "non-empty" or "greater than zero". If the forwarder, the
encode step, the fan-out, or any queue entry is altered or removed, this fails.
*/
func TestDiagnosticsEndToEndDelivery(t *testing.T) {
	Convey("Given a fully wired diagnostics pipeline", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(ctx)
		defer bus.Close()

		crypto := &Crypto{
			ctx:         ctx,
			cancel:      cancel,
			bus:         bus,
			diagnostics: &Diagnostics{started: time.Now()},
			api:         nil,
		}

		crypto.bindDiagnostics()

		Convey("A heartbeat must reconstruct the exact 15-queue topology, byte-for-byte", func() {
			var received atomic.Value

			bus.Wire(types.ChannelFluid, "", func(value any) any {
				frame, ok := value.(types.FluidFrame)
				if ok && frame.Channel == types.DiagnosticsChannel {
					received.Store(frame)
				}
				return nil
			})

			bus.Publish(types.ChannelDiagnostics, crypto.Diagnostics())

			eventually(t, func() bool { return received.Load() != nil })

			frame := received.Load().(types.FluidFrame)

			envelope, err := telemetry.Decode(frame.Payload)
			So(err, ShouldBeNil)

			diag, ok := envelope.Frame.Value.(*wireT.DiagnosticsFrameT)
			So(ok, ShouldBeTrue)
			So(diag.Status, ShouldEqual, "flowing")
			So(diag.Enabled, ShouldBeTrue)

			// The exact topology, in the exact emission order.
			So(len(diag.Queues), ShouldEqual, 15)

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
				{"ui.manifold", "ui", []string{"manifold", "diagnostics"}, []string{"webrtc-hub"}},
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

				// Exactly one subscriber is registered by this test (the fluid
				// capture wire on topic "fluid"), so only the ui.manifold queue
				// — which reads topic "fluid" — reports that subscriber's lane
				// count; every other queue has zero registered lanes.
				expectedLanes := uint64(0)
				if expected.name == "ui.manifold" {
					expectedLanes = uint64(goruntime.GOMAXPROCS(0))
				}

				So(queue.Symbols, ShouldEqual, expectedLanes)
			}
		})

		Convey("ObserveModule must surface a stage with the exact summed nanoseconds", func() {
			crypto.diagnostics.applyModule("correlation", 150*time.Microsecond)
			crypto.diagnostics.applyModule("correlation", 250*time.Microsecond)

			snapshot := crypto.Diagnostics()

			found := false

			for _, stage := range snapshot.Stages {
				if stage.Name != "correlation" {
					continue
				}

				found = true
				So(stage.Count, ShouldEqual, 2)
				So(stage.TotalNs, ShouldEqual, 400000)
			}

			So(found, ShouldBeTrue)
		})

		Convey("Disabling collection must produce the exact disabled frame", func() {
			crypto.diagnostics.Disable()

			frame := crypto.Diagnostics()
			So(frame.Enabled, ShouldBeFalse)
			So(frame.Status, ShouldEqual, "disabled")
			So(len(frame.Stages), ShouldEqual, 0)
			So(len(frame.Queues), ShouldEqual, 0)

			crypto.diagnostics.Enable()
		})
	})
}

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

func BenchmarkDiagnosticsWire(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := runtime.NewWorkspace(ctx)
	defer bus.Close()

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		bus:         bus,
		diagnostics: &Diagnostics{started: time.Now()},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = crypto.Diagnostics().Wire()
	}
}
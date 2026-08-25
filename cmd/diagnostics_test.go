package cmd

import (
	"context"
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

		bus := runtime.NewWorkspace(nil)
		defer bus.Close()

		crypto := &Crypto{
			ctx:         ctx,
			cancel:      cancel,
			bus:         bus,
			diagnostics: &Diagnostics{started: time.Now()},
			ui: runtime.ChannelOf[*types.UIFrame](
				bus, types.ChannelUI,
				func(frame *types.UIFrame) string { return "" },
			),
			fluid: runtime.ChannelOf[types.FluidFrame](
				bus, types.ChannelFluid,
				func(frame types.FluidFrame) string { return frame.Channel },
			),
			diagnosticsCh: runtime.ChannelOf[StreamDiagnostics](
				bus, types.ChannelDiagnostics,
				func(diag StreamDiagnostics) string { return "" },
			),
		}
		// Forward each heartbeat to the dashboard frame exactly as
		// bindDiagnostics does, so the test observes the side-effect path
		// without assembling the whole broker stack.
		bus.Observe(types.ChannelDiagnostics, func(_ string, _ string, value any) {
			diag, ok := value.(StreamDiagnostics)

			if !ok {
				return
			}

			wireFrame := diag.Wire()

			crypto.fluid.Publish(types.FluidFrame{
				Channel: types.DiagnosticsChannel,
				Payload: telemetry.Encode(&wireT.FrameT{
					Type:  wireT.FrameDiagnosticsFrame,
					Value: wireFrame,
				}),
			})
			crypto.ui.Publish(&types.UIFrame{
				Type:  wireT.FrameDiagnosticsFrame,
				Value: wireFrame,
			})
		})

		var receivedUI atomic.Int64
		var receivedFluid atomic.Int64

		runtime.ChannelOf[*types.UIFrame](
			bus, types.ChannelUI,
			func(frame *types.UIFrame) string { return "" },
		).Subscribe("test-diag-ui", func(frame *types.UIFrame) error {
			if frame != nil && frame.Type == wireT.FrameDiagnosticsFrame {
				receivedUI.Add(1)
			}
			return nil
		})

		runtime.ChannelOf[types.FluidFrame](
			bus, types.ChannelFluid,
			func(frame types.FluidFrame) string { return frame.Channel },
		).Subscribe("test-diag-fluid", func(frame types.FluidFrame) error {
			if frame.Channel == types.DiagnosticsChannel && len(frame.Payload) > 0 {
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

func BenchmarkDiagnosticsWire(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := runtime.NewWorkspace(nil)
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

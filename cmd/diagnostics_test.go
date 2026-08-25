package cmd

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestDiagnosticsPublishing(t *testing.T) {
	Convey("Given a Crypto instance with diagnostics enabled", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(nil)
		defer bus.Close()

		thesis := types.NewThesis(ctx)

		crypto, err := NewCrypto(ctx, nil, nil, nil, thesis, bus)
		So(err, ShouldBeNil)
		So(crypto, ShouldNotBeNil)

		var receivedUI atomic.Int64
		var receivedFluid atomic.Int64

		uiChannel := runtime.ChannelOf[*types.UIFrame](
			bus, types.ChannelUI,
			func(frame *types.UIFrame) string { return "" },
		)

		uiChannel.Subscribe("test-diag-ui", func(frame *types.UIFrame) error {
			if frame != nil && frame.Type == wire.FrameDiagnosticsFrame {
				receivedUI.Add(1)
			}
			return nil
		})

		fluidChannel := runtime.ChannelOf[types.FluidFrame](
			bus, types.ChannelFluid,
			func(frame types.FluidFrame) string { return frame.Channel },
		)

		fluidChannel.Subscribe("test-diag-fluid", func(frame types.FluidFrame) error {
			if frame.Channel == types.DiagnosticsChannel && len(frame.Payload) > 0 {
				receivedFluid.Add(1)
			}
			return nil
		})

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

	thesis := types.NewThesis(ctx)
	crypto, _ := NewCrypto(ctx, nil, nil, nil, thesis, bus)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = crypto.Diagnostics().Wire()
	}
}

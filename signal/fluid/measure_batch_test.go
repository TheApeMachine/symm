package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
)

func TestFluidflowFeatureBatchFromMeasure(t *testing.T) {
	configureFluidViper()

	Convey("Given laminar stability frames through Measure", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		symbol := "ETH/EUR"
		signal.SetInstrumentTickSize(symbol, 0.01)
		frames := laminarStabilityFrames(symbol)
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		for index, frame := range frames {
			at := base.Add(time.Duration(index) * integrationSpacing()).UnixNano()
			datapoint := marketDatapoint(frame.channel, frame.payload, at)
			_ = signal.Measure(datapoint)
			datapoint.Release()
		}

		state := signal.registry.loadSymbol(symbol)
		So(state, ShouldNotBeNil)

		reading, readingOK := state.Reading()
		So(readingOK, ShouldBeTrue)

		batch := fluidflowFeatureBatch(reading, state.changePct, state.volume)

		Convey("It should emit a fluidflow feature batch", func() {
			So(len(batch), ShouldBeGreaterThan, 0)
			So(batch[0], ShouldBeGreaterThan, 0)
			So(batch[2], ShouldBeGreaterThan, 0)
			So(batch[5], ShouldBeGreaterThanOrEqualTo, batch[0])
		})
	})
}

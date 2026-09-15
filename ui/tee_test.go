package ui

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestNewUITee(t *testing.T) {
	Convey("Given a new UITee constructor", t, func() {
		tee := NewUITee("ui.tee", 1024)

		Convey("It constructs an initialized wait-free Tee in ready state", func() {
			So(tee, ShouldNotBeNil)
			So(tee.Status(), ShouldEqual, runtime.READY)
			So(tee.Next(), ShouldBeNil)
		})
	})
}

func TestUITeePushAndNext(t *testing.T) {
	Convey("Given a UITee receiving streaming measurements", t, func() {
		tee := NewUITee("ui.tee", 64)

		Convey("When empty, Next returns nil immediately without blocking", func() {
			So(tee.Next(), ShouldBeNil)
		})

		Convey("When raw market data is pushed, Next filters it out and returns nil", func() {
			raw := data.NewMeasurement[float64]("websocket", nil)
			raw.Label = "BTC/USD"
			tee.Push(raw)

			So(tee.Next(), ShouldBeNil)
		})

		Convey("When analytical signal is pushed, Next batches and encodes to FlatBuffers frame", func() {
			now := time.Now()
			measurement := data.NewMeasurement[float64]("toxicity", nil)
			measurement.Label = "BTC/USD"
			measurement.At = now

			tee.Push(measurement)

			frame := tee.Next()
			So(frame, ShouldNotBeNil)

			root := wire.GetRootAsMeasurementsFrame(frame, 0)
			So(root, ShouldNotBeNil)
			So(root.RowsLength(), ShouldEqual, 1)

			var row wire.Measurement
			So(root.Rows(&row, 0), ShouldBeTrue)
			So(string(row.Source()), ShouldEqual, "toxicity")
			So(string(row.Symbol()), ShouldEqual, "BTC/USD")

			So(tee.Next(), ShouldBeNil)
		})
	})
}

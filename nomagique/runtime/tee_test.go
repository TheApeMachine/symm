package runtime

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestNewTee(t *testing.T) {
	Convey("Given a new Tee constructor", t, func() {
		tee := NewTee(1024)

		Convey("It constructs an initialized wait-free SPSC ring buffer", func() {
			So(tee, ShouldNotBeNil)
			So(tee.Ring(), ShouldNotBeNil)
			So(tee.Ring().Cap(), ShouldBeGreaterThanOrEqualTo, 1024)
			So(tee.Ring().IsEmpty(), ShouldBeTrue)
		})
	})
}

func TestTeeRegister(t *testing.T) {
	Convey("Given a Tee registering with the runtime", t, func() {
		tee := NewTee(1024)
		measurement := tee.Register()

		Convey("It declares a telemetry envelope and wildcard peer interests", func() {
			So(measurement, ShouldNotBeNil)
			So(measurement.Source, ShouldEqual, "telemetry.tee")
			So(measurement.Metadata["peer-interest"], ShouldEqual, "*")
		})
	})
}

func TestTeeStep(t *testing.T) {
	Convey("Given a Tee node receiving streaming measurements", t, func() {
		tee := NewTee(64)

		Convey("When a nil measurement is stepped", func() {
			result := tee.Step(nil)

			Convey("It returns nil and puts nothing on the ring", func() {
				So(result, ShouldBeNil)
				So(tee.Ring().IsEmpty(), ShouldBeTrue)
			})
		})

		Convey("When a regular measurement is stepped", func() {
			now := time.Now()
			measurement := data.NewMeasurement[float64]("toxicity", nil)
			measurement.Label = "BTC/USD"
			measurement.At = now

			result := tee.Step(measurement)

			Convey("It returns the measurement and enqueues it to the ring", func() {
				So(result, ShouldEqual, measurement)
				So(tee.Ring().IsEmpty(), ShouldBeFalse)

				dequeued, ok := tee.Ring().Get()
				So(ok, ShouldBeTrue)
				So(dequeued.Source, ShouldEqual, measurement.Source)
				So(dequeued.Label, ShouldEqual, measurement.Label)
				So(tee.Ring().IsEmpty(), ShouldBeTrue)
			})
		})

		Convey("When a measurement with populated peers is stepped", func() {
			now := time.Now()
			root := data.NewMeasurement[float64]("telemetry.tee", nil)
			peerFirst := data.NewMeasurement[float64]("hawkes", nil)
			peerFirst.Label = "BTC/USD"
			peerFirst.At = now

			peerSecond := data.NewMeasurement[float64]("cvd", nil)
			peerSecond.Label = "BTC/USD"
			peerSecond.At = now

			root.Peers = []*data.Measurement[float64]{peerFirst, peerSecond}

			result := tee.Step(root)

			Convey("It enqueues all populated peers into the ring", func() {
				So(result, ShouldEqual, root)

				first, okFirst := tee.Ring().Get()
				So(okFirst, ShouldBeTrue)
				So(first.Source, ShouldEqual, peerFirst.Source)
				So(first.Label, ShouldEqual, peerFirst.Label)

				second, okSecond := tee.Ring().Get()
				So(okSecond, ShouldBeTrue)
				So(second.Source, ShouldEqual, peerSecond.Source)
				So(second.Label, ShouldEqual, peerSecond.Label)

				So(tee.Ring().IsEmpty(), ShouldBeTrue)
			})
		})

		Convey("When a filter is configured", func() {
			tee.SetFilter(func(m *data.Measurement[float64]) bool {
				return m.Source != "websocket"
			})

			now := time.Now()
			root := data.NewMeasurement[float64]("telemetry.tee", nil)
			admittedPeer := data.NewMeasurement[float64]("category", nil)
			admittedPeer.Label = "BTC/USD"
			admittedPeer.At = now

			rejectedPeer := data.NewMeasurement[float64]("websocket", nil)
			rejectedPeer.Label = "BTC/USD"
			rejectedPeer.At = now

			root.Peers = []*data.Measurement[float64]{rejectedPeer, admittedPeer}

			result := tee.Step(root)

			Convey("It enqueues only admitted peers", func() {
				So(result, ShouldEqual, root)

				admitted, ok := tee.Ring().Get()
				So(ok, ShouldBeTrue)
				So(admitted.Source, ShouldEqual, "category")

				So(tee.Ring().IsEmpty(), ShouldBeTrue)
			})
		})
	})
}

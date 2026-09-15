package runtime

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestNewTee(t *testing.T) {
	Convey("Given a new Tee constructor", t, func() {
		tee := NewTee[*data.Measurement[float64]]("test.tee", 1024)

		Convey("It constructs an initialized wait-free Tee in ready state", func() {
			So(tee, ShouldNotBeNil)
			So(tee.Next(), ShouldBeNil)
		})
	})
}

func TestTeePushAndNext(t *testing.T) {
	Convey("Given a Tee receiving streaming measurements", t, func() {
		tee := NewTee[*data.Measurement[float64]]("test.tee", 64)

		Convey("When empty, Next returns nil immediately without blocking", func() {
			So(tee.Next(), ShouldBeNil)
		})

		Convey("When a measurement is pushed, Next returns the dequeued value", func() {
			measurement := data.NewMeasurement[float64]("toxicity", nil)
			measurement.Label = "BTC/USD"

			tee.Push(measurement)

			dequeued := tee.Next()
			So(dequeued, ShouldEqual, measurement)
			So(tee.Next(), ShouldBeNil)
		})
	})
}

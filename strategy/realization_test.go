package strategy

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRealizationMeter(t *testing.T) {
	Convey("Given a fresh execution realization meter", t, func() {
		meter := NewRealizationMeter()
		So(meter.AllowsTrading(), ShouldBeTrue)
		So(meter.Reason(), ShouldEqual, "")

		Convey("successful submissions keep trading enabled", func() {
			meter.ObserveSubmission(nil)
			So(meter.AllowsTrading(), ShouldBeTrue)
		})

		Convey("consecutive submission failures trip the circuit breaker", func() {
			errSample := errors.New("insufficient margin")
			meter.ObserveSubmission(errSample)
			meter.ObserveSubmission(errSample)
			So(meter.AllowsTrading(), ShouldBeTrue)

			meter.ObserveSubmission(errSample)
			So(meter.AllowsTrading(), ShouldBeFalse)
			So(meter.Reason(), ShouldEqual, "consecutive execution submission failures exceeded threshold")

			Convey("reset restores trading permission", func() {
				meter.Reset()
				So(meter.AllowsTrading(), ShouldBeTrue)
			})
		})

		Convey("excessive adverse slippage trips the circuit breaker", func() {
			// reference price 100, fill price 101 -> 100 bps slippage (limit is 50 bps)
			meter.ObserveFill(100.0, 101.0, false)
			So(meter.AllowsTrading(), ShouldBeFalse)
			So(meter.Reason(), ShouldEqual, "realized execution slippage exceeded tolerance")
		})

		Convey("acceptable slippage allows trading", func() {
			// reference price 100, fill price 100.01 -> 1 bps slippage
			meter.ObserveFill(100.0, 100.01, false)
			So(meter.AllowsTrading(), ShouldBeTrue)
		})
	})
}

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

		Convey("catastrophic single fill slippage trips the circuit breaker immediately", func() {
			// reference price 100, fill price 102 -> 200 bps slippage (catastrophic bound is 150 bps)
			meter.ObserveFill(100.0, 102.0, false)
			So(meter.AllowsTrading(), ShouldBeFalse)
			So(meter.Reason(), ShouldEqual, "catastrophic single-fill slippage exceeded bound")
			So(meter.VetoTime().IsZero(), ShouldBeFalse)

			Convey("reset restores trading permission", func() {
				meter.Reset()
				So(meter.AllowsTrading(), ShouldBeTrue)
				So(meter.Reason(), ShouldEqual, "")
			})
		})

		Convey("sustained adverse EWMA slippage trips the circuit breaker", func() {
			// 60 bps adverse slippage on first fill -> EWMA becomes 60 bps (> 50 bps max)
			meter.ObserveFill(100.0, 100.60, false)
			So(meter.AllowsTrading(), ShouldBeFalse)
			So(meter.Reason(), ShouldEqual, "realized execution slippage EWMA exceeded tolerance")
		})

		Convey("acceptable slippage allows trading", func() {
			// reference price 100, fill price 100.01 -> 1 bps slippage
			meter.ObserveFill(100.0, 100.01, false)
			So(meter.AllowsTrading(), ShouldBeTrue)
		})
	})
}

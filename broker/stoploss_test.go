package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStoplossArmsAtOnePercent(t *testing.T) {
	Convey("Healthy below arm; Armed at >1% with 0.5% hard floor", t, func() {
		stop := NewStoploss(0)

		stop.Update(0.9)
		So(stop.State, ShouldEqual, StoplossHealthy)
		So(stop.Triggered(), ShouldBeFalse)

		stop.Update(1.1)
		So(stop.State, ShouldEqual, StoplossArmed)
		So(stop.GivebackPnlPct, ShouldEqual, 0.5)
		So(stop.FloorPnLPct, ShouldAlmostEqual, 1.1*(0.5+0.1*0.05), 1e-9)
		So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, 0.5)
	})
}

func TestStoplossHardFloorGuaranteesHalfPercent(t *testing.T) {
	Convey("Just after arm the floor never sits below 0.5%", t, func() {
		stop := NewStoploss(0)

		stop.Update(1.01)
		So(stop.State, ShouldEqual, StoplossArmed)
		So(stop.FloorPnLPct >= 0.5, ShouldBeTrue)

		// Soft oscillation above the hard floor stays armed.
		stop.Update(0.6)
		So(stop.State, ShouldEqual, StoplossArmed)
		So(stop.Triggered(), ShouldBeFalse)

		stop.Update(0.49)
		So(stop.State, ShouldEqual, StoplossTriggered)
	})
}

func TestStoplossTightensKeepWithPeak(t *testing.T) {
	Convey("Each +1% peak above arm adds 5pp keep", t, func() {
		stop := NewStoploss(0)

		stop.Update(2.0)
		So(stop.State, ShouldEqual, StoplossArmed)
		So(stop.GivebackPnlPct, ShouldEqual, 0.5)
		So(stop.FloorPnLPct, ShouldAlmostEqual, 2.0*0.55, 1e-9)

		stop.Update(3.0)
		So(stop.GivebackPnlPct, ShouldEqual, 0.5)
		So(stop.FloorPnLPct, ShouldAlmostEqual, 3.0*0.60, 1e-9)

		// Dip inside the floor stays armed.
		stop.Update(1.9)
		So(stop.State, ShouldEqual, StoplossArmed)

		stop.Update(1.7)
		So(stop.State, ShouldEqual, StoplossTriggered)
	})
}

func TestStoplossLegacyTakeProfitBecomesArmed(t *testing.T) {
	Convey("Wire value 2 folds into Armed on the next update", t, func() {
		stop := NewStoploss(2.0)
		stop.State = stoplossLegacyTakeProfit

		stop.Update(2.0)
		So(stop.State, ShouldEqual, StoplossArmed)
		So(stop.FloorPnLPct, ShouldAlmostEqual, 2.0*0.55, 1e-9)
	})
}

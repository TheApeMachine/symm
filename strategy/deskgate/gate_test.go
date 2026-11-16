package deskgate

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
)

func TestGateLearn(t *testing.T) {
	Convey("Setup", t, func() {
		gate := NewGate(context.Background(), nil)

		feat := map[string]float64{
			"axis_volume": 0.7,
			"axis_day":    0.6,
			"axis_spread": 0.8,
			"axis_book":   0.65,
			"axis_peak":   0.55,
		}

		before := gate.Axes["volume"].Cut
		gate.Learn(feat, true)

		Convey("developed moves volume cut toward the pass score", func() {
			So(gate.Axes["volume"].Cut, ShouldBeGreaterThan, before)
			So(gate.Axes["volume"].Cut, ShouldBeLessThan, feat["axis_volume"])
		})

		Convey("stalled raises cut when score cleared the bar", func() {
			cut := gate.Axes["day"].Cut
			gate.Learn(feat, false)
			So(gate.Axes["day"].Cut, ShouldBeGreaterThanOrEqualTo, cut)
		})
	})
}

func TestDeveloped(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When stoploss peak cleared arm", func() {
			pos := &broker.Position{
				Stoploss: &broker.Stoploss{PeakPnLPct: 1.2, ArmPnlPct: 1.0},
			}
			So(Developed(pos), ShouldBeTrue)
		})
	})
}
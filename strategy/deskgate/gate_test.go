package deskgate

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

func TestGateLearn(t *testing.T) {
	Convey("Setup", t, func() {
		gate := NewGate(context.Background(), nil)

		feat := map[string]float64{
			"tape_recent": 1,
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
			gate := NewGate(context.Background(), nil)
			So(gate.Developed(pos), ShouldBeTrue)
		})
	})
}

func TestOracleFromCold(t *testing.T) {
	Convey("Setup", t, func() {
		if system.Cfg == nil || system.Cfg.Risk == nil {
			system.Cfg = system.NewConfig()
		}

		gate := NewGate(context.Background(), nil)
		So(gate.Axes["day"].ready(), ShouldBeFalse)

		Convey("When session move clears the risk floor before the track is warm", func() {
			_, day, _, _, _ := gate.OracleFrom(map[string]float64{
				"session_move": system.Cfg.Risk.MinMoverChangePct + 1,
				"volume_usd":   system.Cfg.Risk.MinMoverVolumeUSD + 1,
				"spread_pct":   0.5,
				"touch_cash":   5,
				"backing":      system.Cfg.Risk.MinBackingRatio + 0.1,
				"peak_frac":    0.2,
			})

			Convey("Day approves from the static floor", func() {
				So(day, ShouldBeTrue)
			})
		})

		Convey("When session move is flat before the track is warm", func() {
			_, day, _, _, _ := gate.OracleFrom(map[string]float64{
				"session_move": 0,
			})

			Convey("Day vetoes", func() {
				So(day, ShouldBeFalse)
			})
		})
	})
}
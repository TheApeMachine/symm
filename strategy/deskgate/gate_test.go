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

		Convey("borderline stall raises cut; soft revert keeps the gate open", func() {
			cut := gate.Axes["day"].Cut
			gate.Learn(feat, false)
			So(gate.Axes["day"].Cut, ShouldBeGreaterThan, cut)
			So(gate.Axes["day"].Cut, ShouldBeLessThan, system.Cfg.Learning.ThresholdCeil)
		})

		Convey("strong-looking stall eases cut instead of ratcheting toward ceil", func() {
			axis := gate.Axes["volume"]
			axis.Cut = 0.55
			strong := map[string]float64{
				"tape_recent": 1,
				"axis_volume": 0.9,
				"axis_day":    0.9,
				"axis_spread": 0.9,
				"axis_book":   0.9,
				"axis_peak":   0.9,
			}
			before := axis.Cut
			gate.Learn(strong, false)
			So(gate.Axes["volume"].Cut, ShouldBeLessThan, before)
		})
	})
}

func TestStaticPassPeakMissing(t *testing.T) {
	Convey("Setup", t, func() {
		if system.Cfg == nil || system.Cfg.Risk == nil {
			system.Cfg = system.NewConfig()
		}

		gate := NewGate(context.Background(), nil)

		Convey("Missing peak_frac does not seal the peak lamp", func() {
			_, _, _, _, peak := gate.OracleFrom(map[string]float64{
				"session_move": system.Cfg.Risk.MinMoverChangePct + 1,
				"volume_usd":   system.Cfg.Risk.MinMoverVolumeUSD + 1,
				"spread_pct":   0.5,
				"touch_cash":   5,
				"backing":      system.Cfg.Risk.MinBackingRatio + 0.1,
				"peak_frac":    -1,
			})
			So(peak, ShouldBeTrue)
		})

		Convey("Unknown buy tape passes volume on 24h turnover alone", func() {
			volume, _, _, _, _ := gate.OracleFrom(map[string]float64{
				"volume_usd":    system.Cfg.Risk.MinMoverVolumeUSD + 1,
				"tape_known":    0,
				"tape_notional": 0,
				"tape_ratio":    0,
			})
			So(volume, ShouldBeTrue)
		})

		Convey("Known dead buy tape still fails volume", func() {
			volume, _, _, _, _ := gate.OracleFrom(map[string]float64{
				"volume_usd":    system.Cfg.Risk.MinMoverVolumeUSD + 1,
				"tape_known":    1,
				"tape_notional": 1,
				"tape_ratio":    0.5,
			})
			So(volume, ShouldBeFalse)
		})
	})
}

func TestOraclePassIgnoresRestoredCutWithoutLearn(t *testing.T) {
	Convey("Setup", t, func() {
		if system.Cfg == nil || system.Cfg.Risk == nil {
			system.Cfg = system.NewConfig()
		}

		gate := NewGate(context.Background(), nil)
		axis := gate.Axes["volume"]
		axis.Cut = 0.95
		axis.Sample = 0
		for i := 0; i < 16; i++ {
			_, _ = axis.track.Measure(float64(i) + 1)
		}
		So(axis.ready(), ShouldBeTrue)

		Convey("Warm track with Sample 0 still clears on static floors", func() {
			volume, _, _, _, _ := gate.OracleFrom(map[string]float64{
				"volume_usd":    system.Cfg.Risk.MinMoverVolumeUSD + 1,
				"tape_known":    0,
				"tape_notional": 0,
				"axis_volume":   0.1,
			})
			So(volume, ShouldBeTrue)
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

		Convey("When the track is warm but session move fails the risk floor", func() {
			dayAxis := gate.Axes["day"]
			for i := 0; i < 8; i++ {
				_, _ = dayAxis.track.Measure(float64(i) + 1)
			}
			So(dayAxis.ready(), ShouldBeTrue)
			dayAxis.Cut = 0

			_, day, _, _, _ := gate.OracleFrom(map[string]float64{
				"session_move": 0,
				"axis_day":     1,
			})

			Convey("Static floors still veto under a permissive cut", func() {
				So(day, ShouldBeFalse)
			})
		})
	})
}
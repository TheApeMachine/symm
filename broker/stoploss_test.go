package broker

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStoploss(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When PnL moves through arm and trigger bands", func() {
			cases := []struct {
				name      string
				seed      func(*Stoploss)
				updates   []float64
				wantState StoplossState
			}{
				{
					name:      "healthy stays below arm",
					seed:      func(s *Stoploss) { *s = *NewStoploss(0) },
					updates:   []float64{0.9},
					wantState: StoplossHealthy,
				},
				{
					name:      "exactly at arm threshold stays healthy",
					seed:      func(s *Stoploss) { *s = *NewStoploss(0) },
					updates:   []float64{1.0},
					wantState: StoplossHealthy,
				},
				{
					name: "arms above one percent",
					seed: func(s *Stoploss) { *s = *NewStoploss(0) },
					updates: []float64{0.9, 1.1},
					wantState: StoplossArmed,
				},
				{
					name: "hard floor never sits below half percent after arm",
					seed: func(s *Stoploss) { *s = *NewStoploss(0) },
					updates: []float64{1.01, 0.6, 0.49},
					wantState: StoplossTriggered,
				},
				{
					name: "peak tightening adds keep then triggers",
					seed: func(s *Stoploss) { *s = *NewStoploss(0) },
					updates: []float64{2.0, 3.0, 1.9, 1.7},
					wantState: StoplossTriggered,
				},
				{
					name: "legacy take-profit wire value folds into armed",
					seed: func(s *Stoploss) {
						*s = *NewStoploss(2.0)
						s.State = stoplossLegacyTakeProfit
					},
					updates:   []float64{2.0},
					wantState: StoplossArmed,
				},
				{
					name: "triggered state ignores later updates",
					seed: func(s *Stoploss) {
						*s = *NewStoploss(0)
						s.State = StoplossTriggered
						s.FloorPnLPct = 1
					},
					updates:   []float64{5.0, -1.0},
					wantState: StoplossTriggered,
				},
			}

			for _, tc := range cases {
				Convey("And "+tc.name, func() {
					stop := NewStoploss(0)
					tc.seed(stop)

					for _, pct := range tc.updates {
						stop.Update(pct)
					}

					So(stop.State, ShouldEqual, tc.wantState)
					So(stop.Triggered(), ShouldEqual, tc.wantState == StoplossTriggered)
				})
			}
		})

		Convey("When armed, floor math follows peak keep", func() {
			stop := NewStoploss(0)
			stop.Update(1.1)
			want := 1.1 * (0.5 + 0.1*0.05)

			So(stop.State, ShouldEqual, StoplossArmed)
			So(stop.GivebackPnlPct, ShouldEqual, 0.5)
			So(math.Abs(stop.FloorPnLPct-want), ShouldBeLessThan, 1e-9)
			So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, 0.5)

			stop.Update(2.0)
			So(math.Abs(stop.FloorPnLPct-2.0*0.55), ShouldBeLessThan, 1e-9)

			stop.Update(3.0)
			So(math.Abs(stop.FloorPnLPct-3.0*0.60), ShouldBeLessThan, 1e-9)
		})

		Convey("When wire knobs were zero ensureDefaults restores arm math", func() {
			stop := &Stoploss{
				State:         StoplossHealthy,
				CurrentPnLPct: 0,
				PeakPnLPct:    0,
			}

			stop.Update(1.2)

			So(stop.ArmPnlPct, ShouldEqual, 1.0)
			So(stop.GivebackPnlPct, ShouldEqual, 0.5)
			So(stop.FloorMinPnlPct, ShouldEqual, 0.5)
			So(stop.State, ShouldEqual, StoplossArmed)
		})
	})
}

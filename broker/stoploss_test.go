package broker

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestStoplossAbsoluteHardFloor(t *testing.T) {
	Convey("A mark at or below MaxLossPct triggers without ever clearing profit", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5

		stop := NewStoploss(-1.6)
		So(stop.MaxLossPct, ShouldEqual, -2.5)
		So(stop.ProfitTargetPct, ShouldEqual, 2.5)
		So(stop.FloorPnLPct, ShouldEqual, -2.5)

		feedStoploss(stop, []float64{-1.8, -2.1, -2.5})
		So(stop.Triggered(), ShouldBeTrue)
		So(stop.State, ShouldEqual, StoplossTriggered)
		So(stop.PeakPnLPct, ShouldBeLessThan, stop.ProfitTargetPct)
		assertInvariant(stop)
	})
}

func TestStoplossGreenBelowProfitStaysAlive(t *testing.T) {
	Convey("Marks that stay under ProfitTargetPct and above the floor do not trigger", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5

		stop := NewStoploss(-1.6)
		path := []float64{-1.4, -1.0, -0.5, 0.2, 1.0, 1.5, 2.0, 1.2, 0.8, 1.8}

		fed := feedStoploss(stop, path)
		So(fed, ShouldEqual, len(path))
		So(stop.Triggered(), ShouldBeFalse)
		So(stop.State, ShouldNotEqual, StoplossProtected)
		So(stop.PeakPnLPct, ShouldBeLessThan, stop.ProfitTargetPct)
		assertInvariant(stop)
	})
}

func TestStoplossProfitProtectionFloor(t *testing.T) {
	Convey("Clearing ProfitTargetPct locks a non-negative floor; falling through it triggers", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5

		stop := NewStoploss(-1.6)
		feedStoploss(stop, []float64{-1.0, 1.0, 2.5})
		So(stop.Triggered(), ShouldBeFalse)
		So(stop.State, ShouldEqual, StoplossProtected)
		So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, 0.0)

		feedStoploss(stop, []float64{-0.1})
		So(stop.Triggered(), ShouldBeTrue)
		assertInvariant(stop)
	})
}

func TestStoplossTightensOnDownWidensOnUp(t *testing.T) {
	Convey("A down-move lifts Floor above MaxLoss; a following up-move lowers it again", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5

		stop := NewStoploss(-1.6)
		feedStoploss(stop, []float64{-2.0})
		So(stop.Triggered(), ShouldBeFalse)

		tight := stop.FloorPnLPct
		So(tight, ShouldBeGreaterThan, stop.MaxLossPct)
		// Tightening cannot jump through the mark that caused it.
		So(tight, ShouldBeLessThan, -2.0)

		feedStoploss(stop, []float64{-1.2})
		So(stop.Triggered(), ShouldBeFalse)
		So(stop.FloorPnLPct, ShouldBeLessThan, tight)
		So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, stop.MaxLossPct)
		assertInvariant(stop)
	})
}

/*
pnlTape generates a deterministic PnL% path. chaos in [0,1] scales noise;
drift steers the mean; step is the base move size.
*/
type pnlTape struct {
	seed  uint64
	pnl   float64
	chaos float64
	drift float64
	step  float64
}

func newPnlTape(seed uint64, entry, chaos, drift, step float64) *pnlTape {
	return &pnlTape{
		seed:  seed,
		pnl:   entry,
		chaos: math.Max(0, math.Min(1, chaos)),
		drift: drift,
		step:  step,
	}
}

func (tape *pnlTape) next() float64 {
	u := tape.unit()
	tape.pnl += tape.drift + tape.step*tape.chaos*(2*u-1)
	return tape.pnl
}

func (tape *pnlTape) take(n int) []float64 {
	out := make([]float64, 0, n)
	for range n {
		out = append(out, tape.next())
	}
	return out
}

func (tape *pnlTape) setRegime(chaos, drift, step float64) {
	tape.chaos = math.Max(0, math.Min(1, chaos))
	tape.drift = drift
	tape.step = step
}

func (tape *pnlTape) unit() float64 {
	x := tape.seed
	if x == 0 {
		x = 0x9e3779b97f4a7c15
	}
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	tape.seed = x
	return float64(x) / float64(^uint64(0))
}

/*
feedStoploss pushes every mark through Update. After Triggered, Update is a
no-op; we still walk the full path so regime-length claims are honest.
*/
func feedStoploss(stop *Stoploss, path []float64) int {
	for i, pnl := range path {
		stop.Update(pnl)
		assertInvariant(stop)
		_ = i
	}
	return len(path)
}

/*
assertInvariant is the live rail check after every Update:
  - floor never below MaxLossPct
  - if triggered, mark is at or below floor
  - if not triggered, mark is strictly above floor
*/
func assertInvariant(stop *Stoploss) {
	So(math.IsNaN(stop.FloorPnLPct), ShouldBeFalse)
	So(math.IsInf(stop.FloorPnLPct, 0), ShouldBeFalse)
	So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, stop.MaxLossPct)
	So(stop.PeakPnLPct, ShouldBeGreaterThanOrEqualTo, stop.InitialPnLPct)

	if stop.Triggered() {
		So(stop.State, ShouldEqual, StoplossTriggered)
		So(stop.CurrentPnLPct, ShouldBeLessThanOrEqualTo, stop.FloorPnLPct+1e-9)
		return
	}

	So(stop.CurrentPnLPct, ShouldBeGreaterThan, stop.FloorPnLPct)

	if stop.State == StoplossProtected {
		So(stop.PeakPnLPct, ShouldBeGreaterThanOrEqualTo, stop.ProfitTargetPct)
		So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, 0)
	}
}

func pathMin(path []float64) float64 {
	min := path[0]
	for _, pnl := range path[1:] {
		if pnl < min {
			min = pnl
		}
	}
	return min
}

func TestStoplossTapeChaosLevels(t *testing.T) {
	Convey("Long tapes at rising chaos: rails hold; breach of MaxLoss must trigger", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5
		entry := -1.6
		maxLoss := -system.Cfg.Risk.MaxAllowedLossPct

		cases := []struct {
			name  string
			chaos float64
			drift float64
			step  float64
		}{
			{name: "calm grind up", chaos: 0.05, drift: 0.004, step: 0.02},
			{name: "mild chop", chaos: 0.25, drift: 0.001, step: 0.05},
			{name: "noisy mean-revert", chaos: 0.55, drift: 0, step: 0.08},
			{name: "violent", chaos: 0.90, drift: -0.002, step: 0.12},
		}

		seeds := []uint64{0xC0FFEE, 0xDEADBEEF, 0xA11CE, 0xBADC0DE}

		for _, tc := range cases {
			Convey(tc.name, func() {
				for _, seed := range seeds {
					tape := newPnlTape(seed+uint64(len(tc.name)), entry, tc.chaos, tc.drift, tc.step)
					path := tape.take(400)

					stop := NewStoploss(entry)
					fed := feedStoploss(stop, path)
					So(fed, ShouldEqual, len(path))

					if pathMin(path) <= maxLoss {
						So(stop.Triggered(), ShouldBeTrue)
					}
				}
			})
		}
	})
}

func TestStoplossTapeMustStopOnCrash(t *testing.T) {
	Convey("A gradual crash triggers against the active floor before or at MaxLoss", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5

		stop := NewStoploss(-1.6)
		path := []float64{-1.7, -2.0, -2.4, -2.5, -5.0, -10.0}
		fed := feedStoploss(stop, path)

		So(fed, ShouldEqual, len(path))
		So(stop.Triggered(), ShouldBeTrue)
		So(stop.CurrentPnLPct, ShouldBeLessThanOrEqualTo, stop.FloorPnLPct+1e-9)
		So(stop.FloorPnLPct, ShouldBeGreaterThanOrEqualTo, stop.MaxLossPct)
	})
}

func TestStoplossTapeGapPastHardFloor(t *testing.T) {
	Convey("A sudden gap past MaxLossPct triggers at or below MaxLoss", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5

		stop := NewStoploss(-1.6)
		path := []float64{-2.6, -5.0, -10.0}
		fed := feedStoploss(stop, path)

		So(fed, ShouldEqual, len(path))
		So(stop.Triggered(), ShouldBeTrue)
		So(stop.CurrentPnLPct, ShouldBeLessThanOrEqualTo, stop.MaxLossPct)
	})
}

func TestStoplossTapeRegimeCycle(t *testing.T) {
	Convey("Full calm→dump→squeeze→chop→settle path is fed mark-by-mark with live invariants", t, func() {
		system.Cfg.Risk.MaxAllowedLossPct = 2.5
		entry := -1.6
		maxLoss := -system.Cfg.Risk.MaxAllowedLossPct

		tape := newPnlTape(0xA11CE, entry, 0.1, 0.003, 0.03)
		regimes := []struct {
			name  string
			chaos float64
			drift float64
			step  float64
			n     int
		}{
			{name: "calm", chaos: 0.08, drift: 0.005, step: 0.025, n: 120},
			{name: "dump", chaos: 0.35, drift: -0.008, step: 0.06, n: 100},
			{name: "squeeze", chaos: 0.20, drift: 0.010, step: 0.04, n: 120},
			{name: "chop", chaos: 0.70, drift: 0, step: 0.10, n: 150},
			{name: "settle", chaos: 0.15, drift: 0.004, step: 0.03, n: 100},
		}

		path := make([]float64, 0, 590)
		for _, regime := range regimes {
			tape.setRegime(regime.chaos, regime.drift, regime.step)
			path = append(path, tape.take(regime.n)...)
		}
		So(len(path), ShouldEqual, 590)

		stop := NewStoploss(entry)
		fed := feedStoploss(stop, path)
		So(fed, ShouldEqual, 590)

		if pathMin(path) <= maxLoss {
			So(stop.Triggered(), ShouldBeTrue)
		}
	})
}

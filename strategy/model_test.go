package strategy

import (
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

func TestApplyActionExitIgnoresUnderwaterLots(t *testing.T) {
	Convey("Policy exit does not settle a lot still in loss", t, func() {
		tree := emptyTree()
		model := &Model{
			tree:      &atomic.Pointer[Tree]{},
			resetPath: func(string) {},
		}
		model.tree.Store(tree)

		loss := decimal.NewFromFloat(-0.07)
		lot := &broker.Position{
			Symbol:    "XLM/USD",
			Book:      broker.TierTypeVirtual.String(),
			EntryPath: "R00/R01",
			PnLPct:    &loss,
		}

		var got *runtime.Decision
		decide := func(d *runtime.Decision) { got = d }

		model.applyAction(
			decide, "XLM/USD", "R00/M1", ActionExit, 0.5, false, false,
			[]*broker.Position{lot},
		)

		So(got, ShouldNotBeNil)
		So(got.Action, ShouldEqual, "hold")
		So(got.Gate, ShouldEqual, "pnl")
		So(lot.ClosedAt, ShouldBeNil)
		So(lot.ExitPath, ShouldEqual, "")
	})

	Convey("Policy exit does not settle a thin green below the profit floor", t, func() {
		tree := emptyTree()
		model := &Model{
			tree:      &atomic.Pointer[Tree]{},
			resetPath: func(string) {},
		}
		model.tree.Store(tree)

		thin := decimal.NewFromFloat(0.2)
		lot := &broker.Position{
			Symbol:    "XLM/USD",
			Book:      broker.TierTypeVirtual.String(),
			EntryPath: "R00/R01",
			PnLPct:    &thin,
		}

		var got *runtime.Decision
		decide := func(d *runtime.Decision) { got = d }

		model.applyAction(
			decide, "XLM/USD", "R00/M1", ActionExit, 0.5, false, false,
			[]*broker.Position{lot},
		)

		So(got, ShouldNotBeNil)
		So(got.Action, ShouldEqual, "hold")
		So(got.Gate, ShouldEqual, "pnl")
		So(lot.ExitPath, ShouldEqual, "")
	})

	Convey("Policy exit settles a lot at or above the profit floor", t, func() {
		tree := emptyTree()
		model := &Model{
			tree:      &atomic.Pointer[Tree]{},
			resetPath: func(string) {},
		}
		model.tree.Store(tree)

		win := decimal.NewFromFloat(0.5)
		lot := &broker.Position{
			Symbol:    "XLM/USD",
			Book:      broker.TierTypeVirtual.String(),
			EntryPath: "R00/R01",
			PnLPct:    &win,
		}

		var got *runtime.Decision
		decide := func(d *runtime.Decision) { got = d }

		model.applyAction(
			decide, "XLM/USD", "R00/M1", ActionExit, 0.5, false, false,
			[]*broker.Position{lot},
		)

		So(got, ShouldNotBeNil)
		So(got.Action, ShouldEqual, "exit")
		So(lot.ExitPath, ShouldEqual, "R00/M1")
	})
}

func TestExitSeedsTreeFromPendingSettle(t *testing.T) {
	Convey("stoploss pending → Exit Ensure-writes enter/exit leaves on an empty tree", t, func() {
		tree := emptyTree()
		model := &Model{tree: &atomic.Pointer[Tree]{}}
		model.tree.Store(tree)

		entry := "R06.R00/R07.R01/R08.R02"
		exit := "R09.R03/R10.R04/R11.R05"
		pnl := decimal.NewFromFloat(-0.04)

		closed := time.Now().UTC()
		position := &broker.Position{
			Symbol:    "AKE/USD",
			EntryPath: entry,
			HoldPaths: []string{exit},
			PnL:       &pnl,
			ClosedAt:  &closed,
		}

		// exitCut is what flushSettled uses when the lot sits in pending.
		path := model.exitCut(nil, position.Symbol, position)
		So(path, ShouldEqual, exit)

		model.Exit(position, path)

		enterLeaf, ok := get(tree, entry)
		So(ok, ShouldBeTrue)
		So(enterLeaf.Action, ShouldEqual, "enter")
		// Loss widens resistance (+step off the seed).
		So(enterLeaf.Resistance, ShouldAlmostEqual, seedResistance()+0.05, 0.001)

		exitLeaf, ok := get(tree, exit)
		So(ok, ShouldBeTrue)
		So(exitLeaf.Action, ShouldEqual, "exit")
		So(exitLeaf.Resistance, ShouldAlmostEqual, seedResistance()+0.05, 0.001)

		So(position.ExitPath, ShouldEqual, exit)
	})

	Convey("closed pending trains once — later cuts do not spawn EXIT leaves", t, func() {
		tree := emptyTree()
		model := &Model{tree: &atomic.Pointer[Tree]{}}
		model.tree.Store(tree)

		entry := "R06.R00/R07.R01/R08.R02"
		first := "R09.R03/R10.R04/R11.R05"
		second := "R12.R06/R13.R07/R14.R08"
		win := decimal.NewFromFloat(0.04)
		pnlPct := decimal.NewFromFloat(5.0)
		closed := time.Now().UTC()
		opened := closed.Add(-time.Second)

		position := &broker.Position{
			Symbol:    "AKE/USD",
			Book:      broker.TierTypeVirtual.String(),
			EntryPath: entry,
			PnL:       &win,
			PnLPct:    &pnlPct,
			OpenedAt:  opened,
			ClosedAt:  &closed,
		}

		model.Exit(position, first)
		model.Exit(position, second)

		So(position.ExitPath, ShouldEqual, first)

		delta := model.resistanceDelta(position)
		enterLeaf, ok := get(tree, entry)
		So(ok, ShouldBeTrue)
		So(enterLeaf.Resistance, ShouldAlmostEqual, seedResistance()+delta, 0.001)

		_, ok = get(tree, second)
		So(ok, ShouldBeFalse)
	})

	Convey("exitCut refuses EntryPath — pending must wait for a distinct cut or hold", t, func() {
		model := &Model{}
		position := &broker.Position{
			Symbol:    "AKE/USD",
			EntryPath: "R06.R00/R07.R01",
		}

		cut := tokensAt("R06.R00/R07.R01", 1.0)
		envelope := &runtime.Envelope{
			Paths: map[string]*runtime.Tokens{
				"AKE/USD": &cut,
			},
		}

		So(model.exitCut(envelope, "AKE/USD", position), ShouldEqual, "")
	})
}

func TestResistanceDelta(t *testing.T) {
	Convey("losses raise by the full step", t, func() {
		model := &Model{}
		loss := decimal.NewFromFloat(-0.1)

		So(model.resistanceDelta(&broker.Position{PnL: &loss}), ShouldEqual, system.Cfg.Learning.ThresholdStep)
		So(model.resistanceDelta(&broker.Position{}), ShouldEqual, system.Cfg.Learning.ThresholdStep)
	})

	Convey("short hold + fat PnL% lowers more than long hold + thin PnL%", t, func() {
		model := &Model{}
		win := decimal.NewFromFloat(1)
		step := system.Cfg.Learning.ThresholdStep
		halfLife := system.Cfg.Learning.ResistanceHoldHalfLifeSec
		pnlRef := system.Cfg.Learning.ResistancePnLRefPct
		closed := time.Now().UTC()

		fat := decimal.NewFromFloat(2.0)
		short := &broker.Position{
			PnL:      &win,
			PnLPct:   &fat,
			OpenedAt: closed.Add(-time.Second),
			ClosedAt: &closed,
		}

		thin := decimal.NewFromFloat(0.2)
		long := &broker.Position{
			PnL:      &win,
			PnLPct:   &thin,
			OpenedAt: closed.Add(-2 * time.Duration(halfLife) * time.Second),
			ClosedAt: &closed,
		}

		shortDelta := model.resistanceDelta(short)
		longDelta := model.resistanceDelta(long)

		So(shortDelta, ShouldBeLessThan, 0)
		So(longDelta, ShouldBeLessThan, 0)
		So(shortDelta, ShouldBeLessThan, longDelta)
		So(shortDelta, ShouldBeGreaterThanOrEqualTo, -step)

		// One-second hold is near-full time credit against a multi-hour half-life.
		hold := 1.0
		want := -step * (1.0 / (1.0 + hold/halfLife)) * math.Tanh(2.0/pnlRef)
		So(shortDelta, ShouldAlmostEqual, want, 0.0001)

		hour := &broker.Position{
			PnL:      &win,
			PnLPct:   &fat,
			OpenedAt: closed.Add(-time.Hour),
			ClosedAt: &closed,
		}
		hourDelta := model.resistanceDelta(hour)
		wantHour := -step * (1.0 / (1.0 + 3600.0/halfLife)) * math.Tanh(2.0/pnlRef)
		So(hourDelta, ShouldAlmostEqual, wantHour, 0.0001)
		// More negative = more lowering: short < hour < long (long is thin+slow).
		So(shortDelta, ShouldBeLessThan, hourDelta)
		So(hourDelta, ShouldBeLessThan, longDelta)
	})
}

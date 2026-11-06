package strategy

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
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
		closed := time.Now().UTC()

		position := &broker.Position{
			Symbol:    "AKE/USD",
			Book:      broker.TierTypeVirtual.String(),
			EntryPath: entry,
			PnL:       &win,
			ClosedAt:  &closed,
		}

		model.Exit(position, first)
		model.Exit(position, second)

		So(position.ExitPath, ShouldEqual, first)

		enterLeaf, ok := get(tree, entry)
		So(ok, ShouldBeTrue)
		So(enterLeaf.Resistance, ShouldAlmostEqual, seedResistance()-0.05, 0.001)

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

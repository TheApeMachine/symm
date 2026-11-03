package strategy

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
)

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
		// Loss widens resistance (+0.05 off the seed).
		So(enterLeaf.Resistance, ShouldAlmostEqual, seedResistance+0.05, 0.001)

		exitLeaf, ok := get(tree, exit)
		So(ok, ShouldBeTrue)
		So(exitLeaf.Action, ShouldEqual, "exit")
		So(exitLeaf.Resistance, ShouldAlmostEqual, seedResistance+0.05, 0.001)

		So(position.ExitPath, ShouldEqual, exit)
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

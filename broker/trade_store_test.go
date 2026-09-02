package broker

import (
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestLoadOpenDecision(t *testing.T) {
	Convey("Given an open trade whose entry arbitration was journaled", t, func() {
		store, err := NewPositionStore(filepath.Join(t.TempDir(), "positions.db"))
		So(err, ShouldBeNil)
		defer store.Close()

		position := closeFillPosition("100000", "0.000490", "0.04")
		entryAt := time.Unix(1_788_000_000, 0).UTC()
		position.Holding.EntryAt = &entryAt
		position.Decision.Action = types.ActionEnter
		position.Decision.Reason = "expected move cleared the complete trading cost"
		position.decisionWire = types.DecisionWire(&position.Decision)

		err = store.SaveTrade(position)
		So(err, ShouldBeNil)

		Convey("recovery loads the same frozen decision from SQLite", func() {
			decision, loadErr := store.LoadOpenDecision("SHAPE/USD")

			So(loadErr, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.Id, ShouldEqual, "shape-decision")
			So(decision.Action, ShouldEqual, "enter")
			So(decision.Reason, ShouldEqual, "expected move cleared the complete trading cost")
		})
	})
}

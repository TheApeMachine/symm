package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
TestSyncThesisLifecycle proves the strategy lifecycle surface follows live desk
holdings instead of freezing at submit-time states.
*/
func TestSyncThesisLifecycle(t *testing.T) {
	Convey("Given a thesis lifecycle and live holdings", t, func() {
		thesis := types.NewThesis()

		Convey("It should promote an open lot from entry submitted into managing", func() {
			thesis.NoteLifecycle("BTC/USD", types.LifecycleEntrySubmitted, thesis.At)

			syncThesisLifecycle(thesis, map[string]*types.Holding{
				"BTC/USD": {Symbol: "BTC/USD", Status: types.OPEN},
			})

			phase, found := thesis.Lifecycle.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleManaging)
		})

		Convey("It should mark an exit in progress as partially exited when only part of the lot is left", func() {
			thesis.NoteLifecycle("ETH/USD", types.LifecycleExitSubmitted, thesis.At)

			syncThesisLifecycle(thesis, map[string]*types.Holding{
				"ETH/USD": {Symbol: "ETH/USD", Status: types.PARTIAL_FILLED},
			})

			phase, found := thesis.Lifecycle.Load("ETH/USD")
			So(found, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecyclePartiallyExited)
		})

		Convey("It should close a managing lifecycle once the live holding disappears", func() {
			thesis.NoteLifecycle("SOL/USD", types.LifecycleManaging, thesis.At)

			syncThesisLifecycle(thesis, map[string]*types.Holding{})

			phase, found := thesis.Lifecycle.Load("SOL/USD")
			So(found, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleClosed)
		})
	})
}

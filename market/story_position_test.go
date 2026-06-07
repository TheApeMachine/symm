package market

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// The live position projection must carry the same truths replay's ledger
// projects: fill-derived side and entry, plus peak AND trough ratcheted from
// marks. This is the integration gap the 2026-06-07 audit called priority 1.
func TestStoryPositionStateFromFillFacts(t *testing.T) {
	convey.Convey("Given a story wired to trader fill facts", t, func() {
		entryAt := time.Now().Add(-time.Minute)
		story := &Story{
			positions:    make(map[string]*reasoning.PositionState),
			positionHeld: func(string) bool { return true },
			positionFacts: func(string) (trading.Side, float64, time.Time, bool) {
				return trading.Buy, 100, entryAt, true
			},
			positionStrategy: func(string) string { return "quick_pump" },
		}

		first := story.positionState(types.Measurement{Symbol: "BTC/EUR", Last: 104, At: time.Now()})

		convey.Convey("It should seed entry from the FILL, not the first mark", func() {
			convey.So(first.EntryPrice, convey.ShouldEqual, 100)
			convey.So(first.Side, convey.ShouldEqual, trading.Buy)
			convey.So(first.EntryAt, convey.ShouldEqual, entryAt)
			convey.So(first.Strategy, convey.ShouldEqual, "quick_pump")
			convey.So(first.Peak, convey.ShouldEqual, 104)
			convey.So(first.Trough, convey.ShouldEqual, 104)
		})

		convey.Convey("It should ratchet peak AND trough from subsequent marks", func() {
			story.positionState(types.Measurement{Symbol: "BTC/EUR", Last: 110, At: time.Now()})
			state := story.positionState(types.Measurement{Symbol: "BTC/EUR", Last: 96, At: time.Now()})

			convey.So(state.Peak, convey.ShouldEqual, 110)
			convey.So(state.Trough, convey.ShouldEqual, 96)
			convey.So(state.EntryPrice, convey.ShouldEqual, 100)
		})

		convey.Convey("A flat symbol clears its projection", func() {
			story.positionHeld = func(string) bool { return false }
			state := story.positionState(types.Measurement{Symbol: "BTC/EUR", Last: 96, At: time.Now()})

			convey.So(state.Holding, convey.ShouldBeFalse)
			convey.So(len(story.positions), convey.ShouldEqual, 0)
		})
	})
}

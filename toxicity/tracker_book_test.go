package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestTrackerBookSideDepth(t *testing.T) {
	Convey("Given mid-price observations", t, func() {
		tracker := NewTracker()
		now := time.Now()

		tracker.ObserveMid("BTC/EUR", market.Pair{}, 100)
		tracker.ObserveLast("BTC/EUR", market.Pair{}, 101)

		Convey("It should retain symbol state", func() {
			So(tracker.IsToxic("BTC/EUR", 100, now), ShouldBeFalse)
		})
	})
}

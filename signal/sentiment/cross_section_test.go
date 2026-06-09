package sentiment

import (
	"container/ring"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCrossSectionBreadthStaleness(t *testing.T) {
	Convey("Given one fresh uptick and one stale uptick", t, func() {
		crossSection := &crossSection{
			breadthHistory: ring.New(4),
			matchWindow:    time.Minute,
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		crossSection.universe.Store("FRESH/EUR", changeSnapshot{
			change:    2.0,
			updatedAt: eventAt,
		})
		crossSection.universe.Store("STALE/EUR", changeSnapshot{
			change:    3.0,
			updatedAt: eventAt.Add(-5 * time.Minute),
		})

		Convey("It should ignore stale symbols when computing breadth", func() {
			So(crossSection.breadth(eventAt), ShouldEqual, 1)
		})
	})
}

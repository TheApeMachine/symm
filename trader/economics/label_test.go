package economics

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
)

func TestEntryLabelWithFees(t *testing.T) {
	convey.Convey("Given separate entry and exit fees", t, func() {
		label := EntryLabelWithFees(
			"BTC/EUR",
			"drive",
			"buy",
			broker.Quote{Last: 100, AskDepth: nil},
			10,
			100,
			0.25,
			0.40,
			10,
			time.Now(),
		)

		convey.Convey("It should record the explicit round trip cost", func() {
			convey.So(label.EntryFeePct, convey.ShouldEqual, 0.25)
			convey.So(label.ExitFeePct, convey.ShouldEqual, 0.40)
			convey.So(label.RoundTripCostPct, convey.ShouldAlmostEqual, 0.0075)
		})
	})
}

func TestExitLabelWithFees(t *testing.T) {
	convey.Convey("Given a maker entry closed with a taker exit", t, func() {
		label := ExitLabelWithFees(
			"BTC/EUR",
			"drive",
			100,
			101,
			0.25,
			0.40,
			10,
			time.Now().Add(-time.Second),
			time.Now(),
		)

		convey.Convey("It should subtract the actual entry and exit fees", func() {
			convey.So(label.NetReturn, convey.ShouldAlmostEqual, 0.0025)
		})
	})
}

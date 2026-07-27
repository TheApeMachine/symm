package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
)

/*
TestStoplossTrailMonotonicity proves the production stoploss trail never moves
backward while a held position continues through a favorable bull tape.
*/
func TestStoplossTrailMonotonicity(t *testing.T) {
	Convey("Given a focused bull entry on the production stack", t, func() {
		harness := newDeskHarness(t, 3)
		Reset(harness.reset)
		So(harness.Warmup(), ShouldBeNil)
		symbol := harness.Market.Symbols[0]

		So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
			return nil
		}, symbol), ShouldBeNil)
		So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

		before, err := harness.Wired.Desk.Holding(symbol)
		So(err, ShouldBeNil)
		So(before.Stoploss, ShouldNotBeNil)
		So(before.Stoploss.Floor, ShouldNotBeNil)
		So(before.Stoploss.Peak, ShouldNotBeNil)
		beforeFloor := before.Stoploss.Floor.Copy()
		beforePeak := before.Stoploss.Peak.Copy()

		Convey("A continuing bull trend only raises or preserves floor and peak", func() {
			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				return nil
			}, symbol), ShouldBeNil)

			after, holdErr := harness.Wired.Desk.Holding(symbol)
			So(holdErr, ShouldBeNil)
			So(after.Stoploss, ShouldNotBeNil)
			So(after.Stoploss.Floor, ShouldNotBeNil)
			So(after.Stoploss.Peak, ShouldNotBeNil)
			So(after.Stoploss.Floor.Cmp(beforeFloor), ShouldBeGreaterThanOrEqualTo, 0)
			So(after.Stoploss.Peak.Cmp(beforePeak), ShouldBeGreaterThanOrEqualTo, 0)
			So(after.Stoploss.Peak.Cmp(after.Stoploss.Floor), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

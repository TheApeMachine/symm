package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestStrategyTransitions proves the production stack handles regime transitions
with deterministic entry, selection, and stop-driven exit outcomes.
*/
func TestStrategyTransitions(t *testing.T) {
	Convey("Given a warmed production stack on a three-symbol market", t, func() {
		harness := newDeskHarness(t, 3)
		Reset(harness.reset)
		So(harness.Warmup(), ShouldBeNil)
		symbols := harness.Market.Symbols

		Convey("A spread-compression regime followed by a focused fast breakout enters exactly one position", func() {
			So(harness.Market.Transition(tests.MarketStateSpreadCompression, func() error {
				return nil
			}, symbols[0]), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)

			So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
				return nil
			}, symbols[0]), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			holding, err := harness.Wired.Desk.Holding(symbols[0])
			So(err, ShouldBeNil)
			So(holding.Symbol, ShouldEqual, symbols[0])
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.Mark, ShouldNotBeNil)
		})

		Convey("A leader-follower regime selects the leader symbol and does not open a follower first", func() {
			firstOpen := ""

			So(harness.Market.Transition(tests.MarketStateLeaderFollower, func() error {
				if firstOpen != "" {
					return nil
				}

				for _, candidate := range symbols {
					if _, open := harness.Wired.Desk.Position(candidate); open {
						firstOpen = candidate
						break
					}
				}

				return nil
			}), ShouldBeNil)
			So(firstOpen, ShouldEqual, symbols[0])
		})

		Convey("A sustained bear transition after a bull entry exits the position through the stop path", func() {
			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				return nil
			}, symbols[0]), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			position, ok := harness.Wired.Desk.Position(symbols[0])
			So(ok, ShouldBeTrue)
			So(position.Holding, ShouldNotBeNil)
			So(position.Holding.Stoploss, ShouldNotBeNil)

			So(harness.Market.Transition(tests.MarketStateBearTrend, func() error {
				return nil
			}, symbols[0]), ShouldBeNil)

			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
			So(position.Holding.Status, ShouldEqual, types.CLOSED)
			So(position.Status, ShouldEqual, types.CLOSED)
		})
	})
}

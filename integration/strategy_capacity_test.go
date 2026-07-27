package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestStrategyOccupiedAndCapacity proves the production planner does not re-enter
an already occupied symbol and does not exceed normal slot capacity when a third
challenger arrives.
*/
func TestStrategyOccupiedAndCapacity(t *testing.T) {
	Convey("Given a warmed production stack on a three-symbol market", t, func() {
		Convey("A continued bull trend on an already open symbol emits no duplicate enter", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			symbols := harness.Market.Symbols

			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				return nil
			}, symbols[0]), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			enterCount := 0

			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				for _, decision := range harness.Wired.Thesis.Decisions {
					if decision.Symbol == symbols[0] && decision.Action == types.ActionEnter {
						enterCount++
					}
				}

				return nil
			}, symbols[0]), ShouldBeNil)

			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)
			So(enterCount, ShouldEqual, 0)
		})

		Convey("When two bull symbols fill normal capacity, a third challenger never creates a third open position", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			symbols := harness.Market.Symbols

			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error { return nil }, symbols[0]), ShouldBeNil)
			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error { return nil }, symbols[1]), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, harness.Wired.Desk.MaxSlots(false))

			challengerEnter := 0
			challengerNothing := 0
			challengerCauses := map[string]struct{}{}

			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				for _, decision := range harness.Wired.Thesis.Decisions {
					if decision.Symbol != symbols[2] {
						continue
					}

					challengerCauses[decision.Cause] = struct{}{}

					if decision.Action == types.ActionEnter {
						challengerEnter++
					}

					if decision.Action == types.ActionNothing {
						challengerNothing++
					}
				}

				return nil
			}, symbols[2]), ShouldBeNil)

			So(harness.Wired.Desk.OpenPositions(), ShouldBeLessThanOrEqualTo, harness.Wired.Desk.MaxSlots(false))
			So(challengerEnter+challengerNothing, ShouldBeGreaterThan, 0)

			_, slotsFull := challengerCauses["slots_full"]
			_, rotateWait := challengerCauses["rotate_wait"]
			_, rotation := challengerCauses["rotation"]
			So(slotsFull || rotateWait || rotation, ShouldBeTrue)
		})
	})
}

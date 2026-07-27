package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestStrategyReservedSlots proves fast opportunity trades can consume the two
reserved overflow slots beyond the two normal slots, but never exceed the total
combined capacity.
*/
func TestStrategyReservedSlots(t *testing.T) {
	Convey("Given a warmed production stack on a four-symbol market", t, func() {
		harness := newDeskHarness(t, 4)
		Reset(harness.reset)
		So(harness.Warmup(), ShouldBeNil)

		Convey("Four simultaneous fast pumps can fill all normal plus reserved slots", func() {
			maxOpen := 0

			record := func() error {
				if open := harness.Wired.Desk.OpenPositions(); open > maxOpen {
					maxOpen = open
				}

				return nil
			}

			So(harness.Market.Transition(tests.MarketStateFastPump, record), ShouldBeNil)
			So(harness.Market.Transition(tests.MarketStateFastPump, record), ShouldBeNil)

			So(harness.Wired.Desk.MaxSlots(false), ShouldEqual, 2)
			So(harness.Wired.Desk.MaxSlots(true), ShouldEqual, 4)
			So(maxOpen, ShouldBeLessThanOrEqualTo, 4)

			opportunityCount := 0

			for _, holding := range harness.Wired.Desk.Holdings() {
				if holding.IsOpportunity {
					opportunityCount++
				}
			}

			So(opportunityCount, ShouldBeGreaterThanOrEqualTo, 2)
		})

		Convey("A fifth simultaneous fast pump challenger cannot exceed the combined capacity", func() {
			harness := newDeskHarness(t, 5)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			more := harness.Market.Symbols
			maxOpen := 0

			record := func() error {
				if open := harness.Wired.Desk.OpenPositions(); open > maxOpen {
					maxOpen = open
				}

				return nil
			}

			So(harness.Market.Transition(tests.MarketStateFastPump, record), ShouldBeNil)
			So(harness.Market.Transition(tests.MarketStateFastPump, record), ShouldBeNil)

			So(maxOpen, ShouldBeLessThanOrEqualTo, 4)

			fifthEnter := 0
			fifthNothing := 0
			maxOpen = harness.Wired.Desk.OpenPositions()

			So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
				if open := harness.Wired.Desk.OpenPositions(); open > maxOpen {
					maxOpen = open
				}

				for _, decision := range harness.Wired.Thesis.Decisions {
					if decision.Symbol != more[4] {
						continue
					}

					if decision.Action == types.ActionEnter {
						fifthEnter++
					}

					if decision.Action == types.ActionNothing {
						fifthNothing++
					}
				}

				return nil
			}), ShouldBeNil)

			So(maxOpen, ShouldEqual, 4)
			So(fifthEnter, ShouldEqual, 0)
			_, fifthOpen := harness.Wired.Desk.Position(more[4])
			So(fifthOpen, ShouldBeFalse)
			So(fifthNothing >= 0, ShouldBeTrue)
		})
	})
}

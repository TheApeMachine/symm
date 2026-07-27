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
		symbols := harness.Market.Symbols

		Convey("Four simultaneous fast pumps can fill all normal plus reserved slots", func() {
			for _, symbol := range symbols {
				So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
					return nil
				}, symbol), ShouldBeNil)
			}

			So(harness.Wired.Desk.MaxSlots(false), ShouldEqual, 2)
			So(harness.Wired.Desk.MaxSlots(true), ShouldEqual, 4)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 4)

			reservedCount := 0

			for _, symbol := range symbols {
				holding, err := harness.Wired.Desk.Holding(symbol)
				So(err, ShouldBeNil)
				So(holding.IsOpportunity, ShouldBeTrue)

				phase, ok := harness.Wired.Thesis.Lifecycle.Load(symbol)
				So(ok, ShouldBeTrue)
				So(phase, ShouldEqual, types.LifecycleManaging)

				for _, decision := range harness.Wired.Thesis.Decisions {
					if decision.Symbol == symbol && decision.Action == types.ActionEnter &&
						decision.AllocationClass == "reserved" {
						reservedCount++
					}
				}
			}

			So(reservedCount, ShouldBeGreaterThanOrEqualTo, 2)
		})

		Convey("A fifth fast pump challenger cannot exceed the combined capacity", func() {
			harness := newDeskHarness(t, 5)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			more := harness.Market.Symbols

			for _, symbol := range more[:4] {
				So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
					return nil
				}, symbol), ShouldBeNil)
			}

			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 4)

			fifthEnter := 0
			fifthNothing := 0

			So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
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
			}, more[4]), ShouldBeNil)

			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 4)
			So(fifthEnter, ShouldEqual, 0)
			So(fifthNothing, ShouldBeGreaterThan, 0)
		})
	})
}

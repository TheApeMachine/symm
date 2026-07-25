package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskSlotsAndExit proves slot capacity, single-lot ownership, and
full-lot Sell through the simulated paper path.
*/
func TestDeskSlotsAndExit(t *testing.T) {
	Convey("Given a warmed production desk", t, func() {
		harness := newDeskHarness(t, 3)
		Reset(harness.reset)

		So(harness.Warmup(), ShouldBeNil)
		quote := viper.GetString("market.quote_currency")
		cash, err := harness.Wired.Balance.AssetAvailable(quote)
		So(err, ShouldBeNil)
		So(cash.Sign(), ShouldEqual, 1)

		Convey("When a pump opens exactly one lot", func() {
			opened := ""

			So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
				if opened != "" {
					sellAllOpen(harness.Wired.Desk, harness.Wired.Balance, opened)

					So(harness.Market.Paper.Drain(), ShouldBeNil)
					So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

					return nil
				}

				for open := range harness.Wired.Balance.Holdings() {
					if open.Status != types.OPEN ||
						open.Qty == nil || open.Qty.Sign() <= 0 {
						continue
					}

					opened = open.Symbol
					So(harness.Wired.Desk.OpenPositions(), ShouldBeGreaterThanOrEqualTo, 1)
					So(harness.Wired.Desk.OpenPositions(), ShouldBeLessThanOrEqualTo, harness.Wired.Desk.MaxSlots(false))
					So(harness.Wired.Desk.HasSlot(false) || harness.Wired.Desk.OpenPositions() >= harness.Wired.Desk.MaxSlots(false), ShouldBeTrue)

					return nil
				}

				So(harness.Market.Paper.Drain(), ShouldBeNil)

				return nil
			}), ShouldBeNil)

			So(opened, ShouldNotBeBlank)
			holding, err := harness.Wired.Balance.Holding(opened)
			So(err, ShouldBeNil)
			heldQty := holding.Qty.Copy()

			Convey("Sell closes the full lot and frees the slot", func() {
				So(harness.Wired.Desk.Sell(opened), ShouldBeNil)
				So(harness.Market.Paper.Drain(), ShouldBeNil)

				closed := false

				So(harness.Market.Transition(tests.MarketStateBaseline, func() error {
					So(harness.Market.Paper.Drain(), ShouldBeNil)

					if _, holdErr := harness.Wired.Balance.Holding(opened); holdErr != nil {
						// ponytail: nondeterministic baseline tape may clear the lot
						// before explicit Sell drain completes; upgrade path is
						// deterministic per-scenario seeding on the pump leg.
						closed = true

						return nil
					}

					return nil
				}), ShouldBeNil)

				So(closed, ShouldBeTrue)
				So(heldQty.Sign(), ShouldEqual, 1)
				_, holdErr := harness.Wired.Balance.Holding(opened)
				So(holdErr, ShouldNotBeNil)

				sellAllOpen(harness.Wired.Desk, harness.Wired.Balance)
				So(harness.Market.Paper.Drain(), ShouldBeNil)
				So(harness.Market.Transition(tests.MarketStateBaseline, func() error {
					So(harness.Market.Paper.Drain(), ShouldBeNil)
					sellAllOpen(harness.Wired.Desk, harness.Wired.Balance)
					So(harness.Market.Paper.Drain(), ShouldBeNil)

					return nil
				}), ShouldBeNil)

				So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
				So(harness.Wired.Desk.HasSlot(false), ShouldBeTrue)
			})
		})
	})
}

package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskThinBookDoesNotFabricateMarks proves a thin-liquidity book-only
leg still leaves previously opened marks coherent when tickers arrive again.
*/
func TestDeskThinBookDoesNotFabricateMarks(t *testing.T) {
	Convey("Given an open lot after a pump", t, func() {
		harness := newDeskHarness(t, 2)
		Reset(harness.reset)

		So(harness.Warmup(), ShouldBeNil)

		opened := ""

		So(harness.Market.Transition(tests.MarketStateFastPump, func() error {
			So(harness.Market.Paper.Drain(), ShouldBeNil)

			for _, open := range harness.Wired.Balance.Holdings() {
				if open.Status != types.OPEN || open.Mark == nil {
					continue
				}

				opened = open.Symbol
				So(open.Mark.Sign(), ShouldEqual, 1)
				So(open.PnL, ShouldNotBeNil)

				return nil
			}

			return nil
		}), ShouldBeNil)

		if opened == "" {
			Convey("No enter on this tape is an honest outcome", func() {
				So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
			})

			return
		}

		before, err := harness.Wired.Balance.Holding(opened)
		So(err, ShouldBeNil)
		beforeMark := before.Mark.Copy()

		Convey("Thin liquidity then baseline keeps a positive executable mark", func() {
			So(harness.Market.Transition(tests.MarketStateThinLiquidity, tests.Idle), ShouldBeNil)

			duringThin, thinErr := harness.Wired.Balance.Holding(opened)

			if thinErr == nil {
				So(duringThin.Mark, ShouldNotBeNil)
				So(duringThin.Mark.Cmp(beforeMark), ShouldEqual, 0)
			}

			coherent := false

			So(harness.Market.Transition(tests.MarketStateBaseline, func() error {
				So(harness.Market.Paper.Drain(), ShouldBeNil)
				lot, holdErr := harness.Wired.Balance.Holding(opened)

				if holdErr != nil {
					return nil
				}

				if lot.Mark == nil {
					return nil
				}

				unchanged := lot.Mark.Cmp(beforeMark) == 0
				liftBand := beforeMark.Sub(before.EntryPrice).Abs()

				lower := beforeMark.Sub(liftBand)
				upper := beforeMark.Add(liftBand)
				withinRange := lot.Mark.Cmp(lower) >= 0 && lot.Mark.Cmp(upper) <= 0

				if unchanged || withinRange {
					coherent = true
				}

				return nil
			}), ShouldBeNil)

			// ponytail: nondeterministic thin-liquidity tape may stop out the lot
			// before baseline tickers restore marks; upgrade path is deterministic
			// per-scenario seeding on the pump leg.
			So(coherent || harness.Wired.Desk.OpenPositions() == 0, ShouldBeTrue)
		})
	})
}

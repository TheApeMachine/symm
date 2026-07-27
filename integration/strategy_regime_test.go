package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

type decisionSummary struct {
	enterCount   int
	exitCount    int
	holdCount    int
	nothingCount int
	otherCount   int
	causes       []string
}

func (summary *decisionSummary) observe(thesis *types.Thesis, symbol string) {
	if thesis == nil {
		return
	}

	for _, decision := range thesis.Decisions {
		if decision.Symbol != symbol {
			continue
		}

		summary.causes = append(summary.causes, decision.Cause)

		switch decision.Action {
		case types.ActionEnter:
			summary.enterCount++
		case types.ActionExit:
			summary.exitCount++
		case types.ActionHold:
			summary.holdCount++
		case types.ActionNothing:
			summary.nothingCount++
		default:
			summary.otherCount++
		}
	}
}

/*
TestStrategyRegimes proves the production decision pipeline behaves deterministically
across directional, range-bound, stressed, and black-swan fixture regimes.
*/
func TestStrategyRegimes(t *testing.T) {
	Convey("Given the production stack on a one-symbol simulated market", t, func() {
		harness := newDeskHarness(t, 3)
		Reset(harness.reset)
		So(harness.Warmup(), ShouldBeNil)
		symbol := harness.Market.Symbols[0]

		Convey("A sustained bull trend on the focused symbol ends with exactly one open position", func() {
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			holding, err := harness.Wired.Desk.Holding(symbol)
			So(err, ShouldBeNil)
			So(holding.Symbol, ShouldEqual, symbol)
			So(holding.Qty, ShouldNotBeNil)
			So(holding.Qty.Sign(), ShouldEqual, 1)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryPrice.Sign(), ShouldEqual, 1)
			So(holding.Mark, ShouldNotBeNil)
			So(holding.Mark.Sign(), ShouldEqual, 1)
			So(holding.Stoploss, ShouldNotBeNil)
			So(holding.Stoploss.Entry, ShouldNotBeNil)
			So(holding.Stoploss.Floor, ShouldNotBeNil)
			So(holding.Stoploss.Peak, ShouldNotBeNil)
		})

		Convey("A sustained bear trend opens nothing and never emits an enter decision", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateBearTrend, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.enterCount, ShouldEqual, 0)
			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
		})

		Convey("A sideways range-bound chop opens nothing and never emits an enter decision", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateSidewaysChop, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.enterCount, ShouldEqual, 0)
			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
		})

		Convey("A volatility spike opens nothing and never emits an enter decision", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateVolatilitySpike, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.enterCount, ShouldEqual, 0)
			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
		})

		Convey("A thin-liquidity regime opens nothing and never emits an enter decision", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateThinLiquidity, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.enterCount, ShouldEqual, 0)
			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
		})

		Convey("A sudden reversal opens nothing and never emits an enter decision", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateSuddenReversal, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.enterCount, ShouldEqual, 0)
			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
		})

		Convey("A flash crash from flat buys the oversold rebound instead of staying flat", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}

			So(harness.Market.Transition(tests.MarketStateFlashCrash, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.enterCount, ShouldEqual, 1)
			So(summary.exitCount, ShouldEqual, 0)
			So(summary.otherCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			holding, err := harness.Wired.Desk.Holding(symbol)
			So(err, ShouldBeNil)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.Mark, ShouldNotBeNil)
		})

		Convey("A flash crash after an existing bull entry does not panic liquidate the position", func() {
			harness := newDeskHarness(t, 3)
			Reset(harness.reset)
			So(harness.Warmup(), ShouldBeNil)
			summary := &decisionSummary{}
			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error { return nil }, symbol), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			So(harness.Market.Transition(tests.MarketStateFlashCrash, func() error {
				summary.observe(harness.Wired.Thesis, symbol)
				return nil
			}, symbol), ShouldBeNil)

			So(summary.exitCount, ShouldEqual, 0)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)
		})
	})
}

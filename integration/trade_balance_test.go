package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
)

func assertTradeBalanceIdentity(tradeBalance *kraken.TradeBalanceResult) {
	So(tradeBalance, ShouldNotBeNil)
	So(tradeBalance.Equity, ShouldNotBeNil)
	So(tradeBalance.TradeBalance, ShouldNotBeNil)
	So(tradeBalance.UnrealizedPnL, ShouldNotBeNil)
	So(tradeBalance.CostBasis, ShouldNotBeNil)
	So(tradeBalance.Valuation, ShouldNotBeNil)
	So(tradeBalance.Equity.Cmp(tradeBalance.TradeBalance.Add(tradeBalance.UnrealizedPnL)), ShouldEqual, 0)
	So(tradeBalance.UnrealizedPnL.Cmp(tradeBalance.Valuation.Sub(tradeBalance.CostBasis)), ShouldEqual, 0)
}

/*
TestTradeBalanceLifecycle proves the backend trade-balance surface stays
internally consistent from flat, through entry, and back to flat after exit.
*/
func TestTradeBalanceLifecycle(t *testing.T) {
	Convey("Given a warmed production stack on the simulated market", t, func() {
		harness := newDeskHarness(t, 3)
		Reset(harness.reset)
		So(harness.Warmup(), ShouldBeNil)
		symbol := harness.Market.Symbols[0]

		Convey("The flat account trade balance starts with zero open-position terms", func() {
			tradeBalance, err := harness.Wired.Balance.TradeBalance()
			So(err, ShouldBeNil)
			assertTradeBalanceIdentity(tradeBalance)
			So(tradeBalance.CostBasis.Sign(), ShouldEqual, 0)
			So(tradeBalance.Valuation.Sign(), ShouldEqual, 0)
			So(tradeBalance.UnrealizedPnL.Sign(), ShouldEqual, 0)
			So(tradeBalance.Equity.Cmp(tradeBalance.TradeBalance), ShouldEqual, 0)
		})

		Convey("After a focused bull entry the trade balance exposes non-zero open-position terms", func() {
			So(harness.Market.Transition(tests.MarketStateBullTrend, func() error {
				return nil
			}, symbol), ShouldBeNil)
			So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 1)

			tradeBalance, err := harness.Wired.Balance.TradeBalance()
			So(err, ShouldBeNil)
			assertTradeBalanceIdentity(tradeBalance)
			So(tradeBalance.CostBasis, ShouldNotBeNil)
			So(tradeBalance.CostBasis.Sign(), ShouldEqual, 1)
			So(tradeBalance.Valuation, ShouldNotBeNil)
			So(tradeBalance.Valuation.Sign(), ShouldEqual, 1)
			So(tradeBalance.TradeBalance, ShouldNotBeNil)
			So(tradeBalance.TradeBalance.Sign(), ShouldEqual, 1)
			So(tradeBalance.Equity, ShouldNotBeNil)

			So(tradeBalance.Equity.Cmp(tradeBalance.TradeBalance.Add(tradeBalance.UnrealizedPnL)), ShouldEqual, 0)
			So(tradeBalance.UnrealizedPnL.Cmp(tradeBalance.Valuation.Sub(tradeBalance.CostBasis)), ShouldEqual, 0)

			holding, holdErr := harness.Wired.Desk.Holding(symbol)
			So(holdErr, ShouldBeNil)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.Mark, ShouldNotBeNil)
			So(holding.PnL, ShouldNotBeNil)

			Convey("After an explicit sell the trade balance returns to a flat account state", func() {
				So(harness.Wired.Desk.Sell(symbol), ShouldBeNil)
				So(harness.Market.Paper.Drain(), ShouldBeNil)
				So(harness.Market.Transition(tests.MarketStateBaseline, func() error {
					So(harness.Market.Paper.Drain(), ShouldBeNil)
					return nil
				}), ShouldBeNil)

				So(harness.Wired.Desk.OpenPositions(), ShouldEqual, 0)
				tradeBalance, err = harness.Wired.Balance.TradeBalance()
				So(err, ShouldBeNil)
				So(tradeBalance.CostBasis.Sign(), ShouldEqual, 0)
				So(tradeBalance.Valuation.Sign(), ShouldEqual, 0)
				So(tradeBalance.UnrealizedPnL.Sign(), ShouldEqual, 0)
				So(tradeBalance.Equity.Cmp(tradeBalance.TradeBalance), ShouldEqual, 0)
			})
		})
	})
}

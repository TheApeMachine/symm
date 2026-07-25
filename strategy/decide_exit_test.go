package strategy_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDecideExitSincerity proves an open pump lot survives phantom spoof and
retreat adverse at full quantity, then fully exits on a sincere FastDump stop.
Spoof and retreat are sibling paths: spoof→dump corrupts the level3 book
fixture chain, so the dump saga rides the retreat path the design allows.
*/
func TestDecideExitSincerity(t *testing.T) {
	Convey("Given a warmed production graph with one pumped lot", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		enteredSymbol := ""
		var enteredQty *decimal.Decimal

		So(market.Transition(tests.MarketStateFastPump, func() error {
			if enteredSymbol != "" {
				for open := range wired.Balance.Holdings() {
					if open.Symbol == enteredSymbol || open.Status != types.OPEN {
						continue
					}

					So(wired.Desk.Sell(open.Symbol), ShouldBeNil)
				}

				So(market.Paper.Drain(), ShouldBeNil)

				return nil
			}

			So(market.Paper.Drain(), ShouldBeNil)

			for open := range wired.Balance.Holdings() {
				if open.Status != types.OPEN ||
					open.Qty == nil || open.Qty.Sign() <= 0 {
					continue
				}

				enteredSymbol = open.Symbol
				enteredQty = open.Qty.Copy()
				So(open.Stoploss, ShouldNotBeNil)

				for extra := range wired.Balance.Holdings() {
					if extra.Symbol == enteredSymbol {
						continue
					}

					So(wired.Desk.Sell(extra.Symbol), ShouldBeNil)
				}

				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 1)

				return nil
			}

			return nil
		}), ShouldBeNil)

		So(enteredSymbol, ShouldNotBeBlank)
		So(enteredQty, ShouldNotBeNil)
		So(enteredQty.Sign(), ShouldEqual, 1)
		So(wired.Desk.OpenPositions(), ShouldEqual, 1)

		Convey("Phantom spoof keeps the full lot open", func() {
			So(market.Transition(tests.MarketStateSpoofLiquidity, func() error {
				So(market.Paper.Drain(), ShouldBeNil)
				assertOpenFull(wired, enteredSymbol, enteredQty)

				return nil
			}), ShouldBeNil)
		})

		Convey("Phantom retreat keeps the lot, then a sincere dump stops out", func() {
			So(market.Transition(tests.MarketStateLiquidityRetreat, func() error {
				So(market.Paper.Drain(), ShouldBeNil)
				assertOpenFull(wired, enteredSymbol, enteredQty)

				return nil
			}), ShouldBeNil)

			exited := false

			So(market.Transition(tests.MarketStateFastDump, func() error {
				if exited {
					return nil
				}

				So(market.Paper.Drain(), ShouldBeNil)
				thesis := wired.Thesis

				if thesis != nil {
					for _, decision := range thesis.Decisions {
						So(allowed(decision.Action), ShouldBeTrue)

						if decision.Symbol != enteredSymbol ||
							decision.Action != types.ActionExit {
							continue
						}

						So(decision.Cause, ShouldEqual, "stop")
						So(decision.ProposedQuantity.Cmp(enteredQty), ShouldEqual, 0)
						exited = true

						return nil
					}
				}

				if _, holdErr := wired.Balance.Holding(enteredSymbol); holdErr != nil {
					exited = true
				}

				return nil
			}), ShouldBeNil)

			So(exited, ShouldBeTrue)
			_, holdErr := wired.Balance.Holding(enteredSymbol)
			So(holdErr, ShouldNotBeNil)
		})
	})
}

func assertOpenFull(
	wired *stack.Stack, symbol string, qty *decimal.Decimal,
) {
	thesis := wired.Thesis

	if thesis != nil {
		for _, decision := range thesis.Decisions {
			So(allowed(decision.Action), ShouldBeTrue)

			if decision.Symbol == symbol {
				So(decision.Action, ShouldNotEqual, types.ActionExit)
			}
		}
	}

	lot, holdErr := wired.Balance.Holding(symbol)
	So(holdErr, ShouldBeNil)
	So(lot.Status, ShouldEqual, types.OPEN)
	So(lot.Qty.Cmp(qty), ShouldEqual, 0)
}

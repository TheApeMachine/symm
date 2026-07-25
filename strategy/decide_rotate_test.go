package strategy_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDecideRotation proves a stronger FastPump challenger against full normal
slots either displaces via rotation exit+enter or refuses with audited
rotate_wait — never a silent no-op.
*/
func TestDecideRotation(t *testing.T) {
	Convey("Given a warmed production graph with full normal slots", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)
		So(wired.Desk.MaxSlots(false), ShouldEqual, 2)

		incumbents := map[string]struct{}{}

		for round := 0; round < 6 && len(incumbents) < 2; round++ {
			So(market.Transition(tests.MarketStateFastPump, func() error {
				So(market.Paper.Drain(), ShouldBeNil)

				for open := range wired.Balance.Holdings() {
					if open.Status != types.OPEN ||
						open.Qty == nil || open.Qty.Sign() <= 0 {
						continue
					}

					incumbents[open.Symbol] = struct{}{}
				}

				return nil
			}), ShouldBeNil)
		}

		So(len(incumbents), ShouldEqual, 2)
		So(wired.Desk.OpenPositions(), ShouldEqual, 2)
		So(wired.Desk.HasSlot(false), ShouldBeFalse)

		held := make([]string, 0, len(incumbents))

		for symbol := range incumbents {
			held = append(held, symbol)
		}

		challenger := ""

		for _, symbol := range market.Symbols {
			if _, taken := incumbents[symbol]; taken {
				continue
			}

			challenger = symbol
			break
		}

		So(challenger, ShouldNotBeBlank)

		rotated := false
		waited := false

		Convey("A stronger FastPump challenger rotates or waits with audit", func() {
			So(market.Transition(tests.MarketStateFastPump, func() error {
				So(market.Paper.Drain(), ShouldBeNil)
				thesis := wired.Thesis

				if thesis == nil {
					return nil
				}

				for _, decision := range thesis.Decisions {
					So(allowed(decision.Action), ShouldBeTrue)

					if decision.Symbol == challenger &&
						decision.Action == types.ActionNothing &&
						decision.Cause == "rotate_wait" {
						So(decision.Alternatives, ShouldNotBeNil)
						So(decision.Reason, ShouldNotBeBlank)
						waited = true
					}

					if decision.Action == types.ActionExit &&
						decision.Cause == "rotation" {
						So(decision.Symbol, ShouldBeIn, held)
						rotated = true
					}

					if decision.Action == types.ActionEnter &&
						decision.Cause == "rotation" &&
						decision.Displaces != "" {
						So(decision.Symbol, ShouldEqual, challenger)
						So(decision.Displaces, ShouldBeIn, held)
						rotated = true
					}
				}

				return nil
			}, challenger), ShouldBeNil)

			So(rotated || waited, ShouldBeTrue)
		})
	})
}

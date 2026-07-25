package strategy_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDecideRefusesTraps proves absorption, low-volume lift, and spoof tapes do
not produce ActionEnter through the production boot graph, while a fast pump
control still can.
*/
func TestDecideRefusesTraps(t *testing.T) {
	traps := []struct {
		name  string
		state tests.MarketState
	}{
		{"absorption", tests.MarketStateVolumeAbsorption},
		{"low-volume lift", tests.MarketStateLowVolumeLift},
		{"spoof", tests.MarketStateSpoofLiquidity},
	}

	Convey("Given warmed production graphs on trap tapes", t, func() {
		for _, trap := range traps {
			Convey("A "+trap.name+" tape never admits a fresh enter", func() {
				market := tests.NewMarket(t.Context(), 3)
				wired, err := stack.NewBooter(t.Context()).Test(market)
				So(err, ShouldBeNil)
				Reset(func() {
					So(wired.Close(), ShouldBeNil)
					market.Close()
				})

				So(market.Warmup(tests.Idle), ShouldBeNil)
				wired.Thesis.Forecasts = nil
				wired.Thesis.Decisions = nil

				enters := 0

				So(market.Transition(trap.state, func() error {
					thesis := wired.Thesis

					if thesis == nil {
						return nil
					}

					for _, decision := range thesis.Decisions {
						So(allowed(decision.Action), ShouldBeTrue)

						if decision.Action == types.ActionEnter {
							enters++
						}
					}

					So(market.Paper.Drain(), ShouldBeNil)

					return nil
				}), ShouldBeNil)

				So(enters, ShouldEqual, 0)
			})
		}

		Convey("A fast pump control can still admit an enter", func() {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			Reset(func() {
				So(wired.Close(), ShouldBeNil)
				market.Close()
			})

			So(market.Warmup(tests.Idle), ShouldBeNil)

			entered := false

			So(market.Transition(tests.MarketStateFastPump, func() error {
				if entered {
					return nil
				}

				So(market.Paper.Drain(), ShouldBeNil)

				for open := range wired.Balance.Holdings() {
					if open.Status == types.OPEN &&
						open.Qty != nil && open.Qty.Sign() > 0 {
						entered = true
						return nil
					}
				}

				thesis := wired.Thesis

				if thesis == nil {
					return nil
				}

				for _, decision := range thesis.Decisions {
					if decision.Action == types.ActionEnter {
						entered = true
						return nil
					}
				}

				return nil
			}), ShouldBeNil)

			So(entered, ShouldBeTrue)
		})
	})
}

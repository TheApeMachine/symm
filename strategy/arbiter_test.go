package strategy_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestArbiterArbitrate(t *testing.T) {
	Convey("Given normal slots are occupied", t, tests.WithOrders(t, []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
	}, cmd.Boot, func(market *tests.Market, system *cmd.System) {
		for range 16 {
			market.Tick()
		}

		quantity := decimal.NewFromFloat64(0.25)

		for _, symbol := range market.Symbols[:system.Desk.MaxPositions()] {
			err := system.Desk.Execute(types.Decision{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           symbol.Pair,
				ProposedQuantity: quantity,
				Risk:             entryRisk(system, symbol.Pair),
			})

			So(err, ShouldBeNil)
		}

		So(system.Desk.OpenSlots(false), ShouldEqual, 0)
		So(system.Desk.OpenSlots(true), ShouldEqual, 2)

		thesis := types.NewThesis(nil)
		decision := types.Decision{
			ID:                uuid.NewString(),
			Action:            types.ActionEnter,
			Symbol:            market.Symbols[2].Pair,
			At:                thesis.At,
			Utility:           1.0,
			OpportunityMargin: 1.0,
		}

		thesis.Decisions.Store(decision.Symbol, &decision)

		arbiter := strategy.NewArbiter(system.Desk)
		arbiter.Arbitrate(thesis)

		retained := 0

		thesis.Decisions.Range(func(key, value any) bool {
			retained++

			return true
		})

		So(retained, ShouldEqual, 1)
		So(decision.Action, ShouldEqual, types.ActionNothing)
		So(decision.Cause, ShouldEqual, "slots_full")
		So(decision.AllocationClass, ShouldNotEqual, "reserved")

		Convey("An upstream strategy exit should be suppressed", func() {
			exitThesis := types.NewThesis(nil)
			exitDecision := types.Decision{
				Action:           types.ActionExit,
				Symbol:           market.Symbols[0].Pair,
				ProposedQuantity: quantity,
			}

			exitThesis.Decisions.Store(exitDecision.Symbol, &exitDecision)

			strategy.NewArbiter(system.Desk).Arbitrate(exitThesis)

			So(exitThesis.Decisions, ShouldHaveLength, 1)
			So(exitDecision.Action, ShouldEqual, types.ActionHold)
			So(exitDecision.Cause, ShouldEqual, "stoploss_only")
			So(exitDecision.ProposedQuantity.Sign(), ShouldEqual, 0)
		})
	}))

	Convey("Given all normal and reserved slots are occupied", t, tests.WithOrders(t, []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		testtypes.NewSymbol("SIM4/USD", 100.0, 5150),
		testtypes.NewSymbol("SIM5/USD", 100.0, 8080),
	}, cmd.Boot, func(market *tests.Market, system *cmd.System) {
		for range 16 {
			market.Tick()
		}

		quantity := decimal.NewFromFloat64(0.25)
		totalSlots := system.Desk.OpenSlots(true)

		for positionIndex, symbol := range market.Symbols[:totalSlots] {
			err := system.Desk.Execute(types.Decision{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           symbol.Pair,
				ProposedQuantity: quantity,
				Opportunity:      positionIndex >= system.Desk.MaxPositions(),
				Risk:             entryRisk(system, symbol.Pair),
			})

			So(err, ShouldBeNil)
		}

		So(system.Desk.OpenSlots(true), ShouldEqual, 0)

		thesis := types.NewThesis(nil)
		decision := types.Decision{
			ID:                uuid.NewString(),
			Action:            types.ActionEnter,
			Symbol:            market.Symbols[4].Pair,
			At:                thesis.At,
			Utility:           1.0,
			Opportunity:       true,
			OpportunityMargin: 1.0,
		}

		thesis.Decisions.Store(decision.Symbol, &decision)

		arbiter := strategy.NewArbiter(system.Desk)
		arbiter.Arbitrate(thesis)

		retained := 0

		thesis.Decisions.Range(func(key, value any) bool {
			retained++

			return true
		})

		So(retained, ShouldEqual, 1)
		So(decision.Action, ShouldEqual, types.ActionNothing)
		So(decision.Cause, ShouldEqual, "slots_full")
	}))

	Convey("Given normal slots are occupied by stop-governed positions", t, tests.WithOrders(t, []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 110.0, 42),
		testtypes.NewSymbol("SIM2/USD", 110.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 110.0, 90210),
	}, cmd.Boot, func(market *tests.Market, system *cmd.System) {
		market.WithAutoFill()

		for range 16 {
			market.Tick()
		}

		quantity := decimal.NewFromFloat64(0.25)

		for _, symbol := range market.Symbols[:system.Desk.MaxPositions()] {
			err := system.Desk.Execute(types.Decision{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           symbol.Pair,
				ProposedQuantity: quantity,
				Risk:             entryRisk(system, symbol.Pair),
			})

			So(err, ShouldBeNil)
		}

		market.Tick()
		market.Tick()

		So(system.Desk.OpenSlots(false), ShouldEqual, 0)

		decisions := make([]types.Decision, 0, system.Desk.MaxPositions()+1)

		for _, symbol := range market.Symbols[:system.Desk.MaxPositions()] {
			decisions = append(decisions, types.Decision{
				Action:  types.ActionHold,
				Symbol:  symbol.Pair,
				Utility: 0.02,
				Alternatives: map[string]float64{
					"hold": 0.02,
					"exit": -0.008,
				},
			})
		}

		Convey("A weaker challenger should wait for a stoploss to free a slot", func() {
			thesis := types.NewThesis(nil)
			for _, decision := range decisions {
				thesis.Decisions.Store(decision.Symbol, &decision)
			}

			decision := types.Decision{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           market.Symbols[2].Pair,
				At:               thesis.At,
				Utility:          0.01,
				ProposedQuantity: quantity,
			}

			thesis.Decisions.Store(decision.Symbol, &decision)

			strategy.NewArbiter(system.Desk).Arbitrate(thesis)

			exits := 0

			thesis.Decisions.Range(func(key, value any) bool {
				decision := value.(*types.Decision)

				if decision.Action == types.ActionExit {
					exits++
				}

				if decision.Symbol == market.Symbols[2].Pair {
					So(decision.Action, ShouldEqual, types.ActionNothing)
					So(decision.Cause, ShouldEqual, "slots_full")
				}

				return true
			})

			So(exits, ShouldEqual, 0)
		})

		Convey("Even a stronger challenger should not liquidate an incumbent", func() {
			thesis := types.NewThesis(nil)
			for _, decision := range decisions {
				thesis.Decisions.Store(decision.Symbol, &decision)
			}

			decision := types.Decision{
				ID:               uuid.NewString(),
				Action:           types.ActionEnter,
				Symbol:           market.Symbols[2].Pair,
				At:               thesis.At,
				Utility:          0.05,
				ProposedQuantity: quantity,
			}

			thesis.Decisions.Store(decision.Symbol, &decision)

			strategy.NewArbiter(system.Desk).Arbitrate(thesis)

			exits := 0

			thesis.Decisions.Range(func(key, value any) bool {
				decision := value.(*types.Decision)

				if decision.Action == types.ActionExit {
					exits++
				}

				if decision.Symbol == market.Symbols[2].Pair {
					So(decision.Action, ShouldEqual, types.ActionNothing)
					So(decision.Cause, ShouldEqual, "slots_full")
					So(decision.Displaces, ShouldBeBlank)
				}

				return true
			})

			So(exits, ShouldEqual, 0)
		})
	}))
}

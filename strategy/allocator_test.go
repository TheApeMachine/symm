package strategy_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func TestAllocatorAllocate(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	Convey(
		"Given a market with positive cash balance",
		t, tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			for range 16 {
				market.Tick()
			}

			allocator := strategy.NewAllocator(
				context.Background(),
				system.Desk.Balance(),
				system.Desk.Instrument(),
				system.Desk.Price(),
				system.Desk,
			)

			Convey("An entry decision should be sized and marked ready", func() {
				thesis := types.NewThesis(nil)
				decision := types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "SIM1/USD",
					At:     thesis.At,
				}

				thesis.Decisions.Store(decision.Symbol, &decision)

				err := allocator.Allocate(thesis)
				So(err, ShouldBeNil)
				So(thesis.Allocator, ShouldBeTrue)
				So(decision.Action, ShouldEqual, types.ActionEnter)
				So(decision.ProposedQuantity, ShouldNotBeNil)
				So(decision.ProposedQuantity.Sign(), ShouldBeGreaterThan, 0)
				So(decision.ProposedNotional, ShouldNotBeNil)
				So(decision.ProposedNotional.Sign(), ShouldBeGreaterThan, 0)
				So(decision.ReferencePrice, ShouldNotBeNil)
			})

			Convey("A sized entry should carry the geometry it was sized under", func() {
				thesis := types.NewThesis(nil)
				decision := types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "SIM1/USD",
					At:     thesis.At,
				}

				thesis.Decisions.Store(decision.Symbol, &decision)

				So(allocator.Allocate(thesis), ShouldBeNil)

				plan := decision.Risk
				So(plan.Present, ShouldBeTrue)
				So(plan.RiskDistance.Sign(), ShouldEqual, 1)
				So(plan.MaxLoss.Sign(), ShouldEqual, 1)

				/*
					The coupling: whatever this entry was sized at, reaching the hard
					floor with it must cost no more than the loss budget. Stop
					distance and quantity are one decision, and a stop wide enough to
					survive noise is only affordable because the size came down to
					meet it.
				*/
				lossPerUnit := plan.LossPerUnit(decision.ReferencePrice)
				So(lossPerUnit, ShouldNotBeNil)
				loss := lossPerUnit.Mul(decision.ProposedQuantity)
				So(loss.Cmp(plan.MaxLoss), ShouldBeLessThanOrEqualTo, 0)
			})

			Convey("A risk distance the budget cannot carry should shrink the quantity", func() {
				thesis := types.NewThesis(nil)
				decision := types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "SIM1/USD",
					At:     thesis.At,
				}

				thesis.Decisions.Store(decision.Symbol, &decision)

				So(allocator.Allocate(thesis), ShouldBeNil)
				unconstrained := decision.ProposedQuantity

				/*
					A wide expected spread widens the boundary, which is exactly the
					case where an unchanged size would turn every stopped trade into
					a proportionally larger loss.
				*/
				wide := types.NewThesis(nil)
				wideDecision := types.Decision{
					ID:             uuid.NewString(),
					Action:         types.ActionEnter,
					Symbol:         "SIM1/USD",
					At:             wide.At,
					ExpectedSpread: decimal.NewFromFloat64(5),
					ExpectedImpact: decimal.NewFromFloat64(1),
					ReferencePrice: decimal.NewFromFloat64(100),
				}

				wide.Decisions.Store(wideDecision.Symbol, &wideDecision)

				So(allocator.Allocate(wide), ShouldBeNil)

				if wideDecision.Action == types.ActionEnter {
					So(wideDecision.Risk.RiskDistance.Cmp(
						decision.Risk.RiskDistance,
					), ShouldEqual, 1)
					So(wideDecision.ProposedQuantity.Cmp(unconstrained), ShouldEqual, -1)
				} else {
					// Or the size it would need falls below what the venue will
					// accept, which is a refusal rather than an oversized bet.
					So(wideDecision.Reason, ShouldEqual,
						"sized quantity below minimum pair order size")
				}
			})

			Convey("An unconfigured pair should be rejected in place", func() {
				thesis := types.NewThesis(nil)
				decision := types.Decision{
					ID:     uuid.NewString(),
					Action: types.ActionEnter,
					Symbol: "UNKNOWN/USD",
					At:     thesis.At,
				}

				thesis.Decisions.Store(decision.Symbol, &decision)

				err := allocator.Allocate(thesis)
				So(err, ShouldBeNil)
				So(decision.Action, ShouldEqual, types.ActionNothing)
				So(decision.Reason, ShouldEqual, "instrument pair unavailable")
			})

			Convey("A published flow haircut should reduce notional before risk sizing", func() {
				clean := types.NewThesis(nil)
				cleanDecision := types.Decision{
					ID:               uuid.NewString(),
					Action:           types.ActionEnter,
					Symbol:           "SIM1/USD",
					At:               clean.At,
					ProposedNotional: decimal.NewFromFloat64(100),
				}

				clean.Decisions.Store(cleanDecision.Symbol, &cleanDecision)
				So(allocator.Allocate(clean), ShouldBeNil)

				reduced := types.NewThesis(nil)
				reducedDecision := types.Decision{
					ID:                      uuid.NewString(),
					Action:                  types.ActionEnter,
					Symbol:                  "SIM1/USD",
					At:                      reduced.At,
					ProposedNotional:        decimal.NewFromFloat64(100),
					AllocationHaircut:       0.5,
					AllocationHaircutReason: "toxicity",
				}

				reduced.Decisions.Store(reducedDecision.Symbol, &reducedDecision)
				So(allocator.Allocate(reduced), ShouldBeNil)

				So(reducedDecision.Action, ShouldEqual, types.ActionEnter)
				So(reducedDecision.ProposedNotional.Cmp(
					cleanDecision.ProposedNotional,
				), ShouldEqual, -1)
				So(reducedDecision.ProposedQuantity.Cmp(
					cleanDecision.ProposedQuantity,
				), ShouldEqual, -1)
			})
		}))
}

func BenchmarkAllocatorAllocate(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
	}

	market := tests.NewMarket(b.Context(), symbols)
	defer market.Close()

	public, private := market.Feeds()
	system := cmd.Boot(b.Context(), types.NewThesis(nil), public, private, nil)

	defer system.Close()

	for range 16 {
		market.Tick()
	}

	allocator := strategy.NewAllocator(
		context.Background(),
		system.Desk.Balance(),
		system.Desk.Instrument(),
		system.Desk.Price(),
		system.Desk,
	)

	thesis := types.NewThesis(nil)
	decision := types.Decision{
		ID:     uuid.NewString(),
		Action: types.ActionEnter,
		Symbol: "SIM1/USD",
		At:     thesis.At,
	}

	thesis.Decisions.Store(decision.Symbol, &decision)

	for b.Loop() {
		decision.Action = types.ActionEnter
		decision.ProposedQuantity = nil
		decision.ProposedNotional = nil
		_ = allocator.Allocate(thesis)
	}
}

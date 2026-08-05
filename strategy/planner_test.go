package strategy_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func stampPlannerReadiness(thesis *types.Thesis) {
	for _, source := range []types.SourceType{
		types.SourceCorrelation,
		types.SourceCVD,
		types.SourceDepthFlow,
		types.SourceExhaustion,
		types.SourceHawkes,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourcePumpDump,
		types.SourceSentiment,
		types.SourceToxicity,
	} {
		thesis.Readiness.Stamp(source)
	}

	thesis.Readiness.Manifold = true
	thesis.Readiness.Resonance = true
	thesis.Readiness.Causal = true
	thesis.Readiness.Graph = true
	thesis.Readiness.Categories = true
	thesis.Readiness.Cognition = true
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a thesis that is not yet ready for strategy evaluation", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("Every planner update should still advance the market tick", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			before := market.Thesis.Tick

			market.Planner.Update(market.Thesis)
			So(market.Thesis.Tick, ShouldEqual, before+1)

			market.Planner.Update(market.Thesis)
			So(market.Thesis.Tick, ShouldEqual, before+2)
		}))
	})

	Convey("Given repeated complete planner passes without a decision", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("It should prepare next evaluation while retaining cycle evidence", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			thesis := market.Thesis
			base := time.Unix(1_700_006_000, 0).UTC()
			thesis.NoteLifecycle("SIM1/USD", types.LifecycleManaging, base)
			stampPlannerReadiness(thesis)

			observedAt := base
			thesis.AppendTicker(kraken.TickerData{
				Symbol: "SIM1/USD", Timestamp: observedAt,
			})
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "SIM1/USD", TradeID: 1,
				Timestamp: observedAt,
			})
			thesis.AppendMeasurements(types.SourceCVD, &types.Measurement{
				Source: types.SourceCVD, Symbol: "SIM1/USD", At: observedAt,
			})

			market.Planner.Update(thesis)

			So(thesis.MarketTickers(), ShouldNotBeEmpty)
			So(thesis.MarketTrades(), ShouldNotBeEmpty)
			So(thesis.Series("SIM1/USD"), ShouldNotBeEmpty)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
		}))
	})

	Convey("Given a deferred opportunity evaluation", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("It should complete evaluation and prepare next pass retaining multi-read history", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			thesis := market.Thesis
			observedAt := time.Unix(1_700_006_050, 0).UTC()
			thesis.AppendTicker(kraken.TickerData{
				Symbol: "SIM1/USD", Timestamp: observedAt,
			})
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "SIM1/USD", TradeID: 1, Timestamp: observedAt,
			})
			thesis.AppendMeasurements(types.SourcePumpDump, &types.Measurement{
				Source: types.SourcePumpDump, Symbol: "SIM1/USD", At: observedAt,
			})
			thesis.Decisions = []types.Decision{{
				Action: types.ActionNothing, Symbol: "SIM1/USD", At: observedAt,
			}}
			stampPlannerReadiness(thesis)

			market.Planner.Update(thesis)

			So(thesis.MarketTickers(), ShouldNotBeEmpty)
			So(thesis.MarketTrades(), ShouldNotBeEmpty)
			So(thesis.Series("SIM1/USD"), ShouldNotBeEmpty)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
		}))
	})

	Convey("Given a completed decision set", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("It should emit the decisions before closing canonical history", tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
			thesis := market.Thesis
			observedAt := time.Unix(1_700_006_100, 0).UTC()
			thesis.AppendTicker(kraken.TickerData{
				Symbol: "SIM1/USD", Timestamp: observedAt,
			})
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "SIM1/USD", TradeID: 1, Timestamp: observedAt,
			})
			thesis.AppendMeasurements(types.SourceCVD, &types.Measurement{
				Source: types.SourceCVD, Symbol: "SIM1/USD", At: observedAt,
			})
			thesis.NoteLifecycle("SIM1/USD", types.LifecycleManaging, observedAt)
			stampPlannerReadiness(thesis)

			market.Planner.Update(thesis)
			So(thesis.Decisions, ShouldBeEmpty)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
			So(market.Thesis, ShouldEqual, thesis)

			stampPlannerReadiness(thesis)
			thesis.Decisions = []types.Decision{{
				Action: types.ActionHold, Symbol: "SIM1/USD", At: observedAt,
			}}
			subscription := market.Planner.Subscribe(
				"cycle-close-test", types.NewSubscription[any](),
			)

			market.Planner.Update(thesis)

			emitted := (<-subscription.Channel).([]types.Decision)
			So(emitted, ShouldHaveLength, 1)
			So(emitted[0].ValidID(), ShouldBeTrue)
			So(thesis.MarketTickers(), ShouldBeEmpty)
			So(thesis.MarketTrades(), ShouldBeEmpty)
			So(thesis.Series("SIM1/USD"), ShouldBeEmpty)
			_, foundLifecycle := thesis.Lifecycle.Load("SIM1/USD")
			So(foundLifecycle, ShouldBeFalse)
		}))
	})
}

func TestPlannerPumpEntry(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
	}

	Convey("Given a fast pump whose forecast never clears executable friction", t, tests.WithMarket(t, symbols, func(market *tests.Market) {
		initialOpportunitySlots := market.Desk.OpenSlots(true)

		for range 64 {
			market.Tick()
		}

		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, testtypes.FastPump), ShouldBeNil)
		}

		for range 256 {
			market.Tick()
		}

		decisions := market.Decisions()
		pricedCandidates := 0
		uneconomicCandidates := 0
		entryDecisions := 0
		maximumExecutableReturn := decimal.NewFromInt64(0)

		for _, decision := range decisions {
			So(decision.ValidID(), ShouldBeTrue)
			So(decision.Symbol, ShouldNotBeBlank)
			So(decision.Cause, ShouldNotBeBlank)
			So(decision.Reason, ShouldNotBeBlank)

			if decision.ExpectedReturn == nil ||
				decision.ExpectedFees == nil ||
				decision.ExpectedSpread == nil ||
				decision.ExpectedImpact == nil {
				continue
			}

			pricedCandidates++
			friction := decision.ExpectedFees.
				Add(decision.ExpectedSpread).
				Add(decision.ExpectedImpact)
			executableReturn := decision.ExpectedReturn.Sub(friction)

			if executableReturn.Cmp(maximumExecutableReturn) > 0 {
				maximumExecutableReturn = executableReturn
			}

			if executableReturn.Sign() <= 0 {
				uneconomicCandidates++
				So(decision.Action, ShouldNotEqual, types.ActionEnter)
			}

			if decision.Utility <= 0 {
				So(decision.Action, ShouldNotEqual, types.ActionEnter)
			}

			if decision.Action != types.ActionEnter {
				continue
			}

			entryDecisions++
			So(executableReturn.Sign(), ShouldEqual, 1)
			So(decision.Utility, ShouldBeGreaterThan, 0.0)
			So(decision.ProposedQuantity, ShouldNotBeNil)
			So(decision.ProposedQuantity.Sign(), ShouldEqual, 1)
			So(decision.ProposedNotional, ShouldNotBeNil)
			So(decision.ProposedNotional.Sign(), ShouldEqual, 1)
			So(decision.ReferencePrice, ShouldNotBeNil)
			So(decision.ReferencePrice.Sign(), ShouldEqual, 1)
			So(decision.Confidence, ShouldBeBetweenOrEqual, 0.0, 1.0)
			So(decision.Uncertainty, ShouldBeGreaterThanOrEqualTo, 0.0)
			So(decision.ForecastSource, ShouldNotBeBlank)
			So(decision.ValidThroughEpoch, ShouldBeGreaterThan, uint64(0))
		}

		So(decisions, ShouldNotBeEmpty)
		So(market.Measurements(), ShouldContainKey, "pumpdump")
		So(pricedCandidates, ShouldBeGreaterThan, 0)
		So(uneconomicCandidates, ShouldEqual, pricedCandidates)
		So(maximumExecutableReturn.Sign(), ShouldBeLessThanOrEqualTo, 0)
		So(entryDecisions, ShouldEqual, 0)
		So(market.Desk.OpenPositions(), ShouldEqual, 0)
		So(market.Desk.OpenSlots(true), ShouldEqual, initialOpportunitySlots)
	}))
}

func TestPlannerSlotDiscipline(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		testtypes.NewSymbol("SIM4/USD", 100.0, 5150),
	}

	Convey("Given simultaneous pump candidates below their own trading costs", t, tests.WithMarket(t, symbols, func(market *tests.Market) {
		initialNormalSlots := market.Desk.OpenSlots(false)
		initialOpportunitySlots := market.Desk.OpenSlots(true)

		for range 64 {
			market.Tick()
		}

		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, testtypes.FastPump), ShouldBeNil)
		}

		for range 256 {
			market.Tick()
		}

		pricedCandidates := 0
		uneconomicCandidates := 0
		entryDecisions := 0
		reservedDecisions := 0
		rounds := map[int64][]types.Decision{}

		for _, decision := range market.Decisions() {
			if decision.AllocationClass == "reserved" {
				reservedDecisions++
			}

			if decision.ExpectedReturn != nil &&
				decision.ExpectedFees != nil &&
				decision.ExpectedSpread != nil &&
				decision.ExpectedImpact != nil {
				pricedCandidates++
				friction := decision.ExpectedFees.
					Add(decision.ExpectedSpread).
					Add(decision.ExpectedImpact)

				if decision.ExpectedReturn.Cmp(friction) <= 0 {
					uneconomicCandidates++
					So(decision.Action, ShouldNotEqual, types.ActionEnter)
					So(decision.AllocationClass, ShouldNotEqual, "reserved")
				}
			}

			if decision.Utility <= 0 {
				So(decision.Action, ShouldNotEqual, types.ActionEnter)
				So(decision.AllocationClass, ShouldNotEqual, "reserved")
			}

			if decision.Action != types.ActionEnter {
				continue
			}

			entryDecisions++
			So(decision.SlotCapacity, ShouldEqual, market.Desk.MaxPositions())
			So(decision.OpenPositions, ShouldBeGreaterThanOrEqualTo, 0)
			So(decision.OpenPositions, ShouldBeLessThanOrEqualTo, decision.SlotCapacity)
			So(decision.ArbitrationRound, ShouldBeGreaterThan, int64(0))
			So(decision.AllocationClass, ShouldBeIn, []string{"normal", "reserved"})
			rounds[decision.ArbitrationRound] = append(rounds[decision.ArbitrationRound], decision)
		}

		for _, round := range rounds {
			normalDecisions := 0
			reservedRoundDecisions := 0

			for _, decision := range round {
				if decision.AllocationClass == "normal" {
					normalDecisions++
				}

				if decision.AllocationClass == "reserved" {
					reservedRoundDecisions++
					So(decision.Opportunity, ShouldBeTrue)
					So(decision.OpportunityMargin, ShouldBeGreaterThan, 0.0)
				}
			}

			So(normalDecisions, ShouldBeLessThanOrEqualTo, initialNormalSlots)
			So(normalDecisions+reservedRoundDecisions, ShouldBeLessThanOrEqualTo, initialOpportunitySlots)

			if reservedRoundDecisions > 0 {
				So(normalDecisions, ShouldEqual, initialNormalSlots)
			}
		}

		So(pricedCandidates, ShouldBeGreaterThan, 0)
		So(uneconomicCandidates, ShouldEqual, pricedCandidates)
		So(entryDecisions, ShouldEqual, 0)
		So(reservedDecisions, ShouldEqual, 0)
		So(market.Desk.OpenPositions(), ShouldEqual, 0)
		So(market.Desk.OpenSlots(false), ShouldEqual, initialNormalSlots)
		So(market.Desk.OpenSlots(true), ShouldEqual, initialOpportunitySlots)
	}))
}

func TestPlannerPumpReversal(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
	}

	Convey("Given an open simulated position when a pump reverses into a dump", t, tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
		market.WithAutoFill()

		for range 64 {
			market.Tick()
		}

		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, testtypes.FastPump), ShouldBeNil)
		}

		for range 64 {
			market.Tick()
		}

		entryQuantity := decimal.NewFromFloat64(0.25)
		So(market.Desk.Execute([]types.Decision{{
			ID:               uuid.NewString(),
			Action:           types.ActionEnter,
			Symbol:           symbols[0].Pair,
			ProposedQuantity: entryQuantity,
			Risk:             tests.EntryRisk(market, symbols[0].Pair),
		}}), ShouldBeNil)

		market.Tick()
		market.Tick()

		positions := slices.Collect(market.Desk.Positions())
		So(positions, ShouldHaveLength, 1)

		position := positions[0]
		So(position.Status, ShouldEqual, types.OPEN)
		So(position.Holding.SellableQty.Cmp(entryQuantity), ShouldEqual, 0)

		reversalDecisionOffset := len(market.Decisions())
		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, testtypes.FastDump), ShouldBeNil)
		}

		for range 256 {
			market.Tick()
		}

		decisions := market.Decisions()
		So(len(decisions), ShouldBeGreaterThanOrEqualTo, reversalDecisionOffset)
		reversalDecisions := decisions[reversalDecisionOffset:]
		strategyExitDecisions := 0
		entryDecisions := 0

		for _, decision := range reversalDecisions {
			if decision.Action == types.ActionEnter {
				entryDecisions++
			}

			if decision.Action != types.ActionExit {
				continue
			}

			strategyExitDecisions++
		}

		So(entryDecisions, ShouldEqual, 0)
		So(strategyExitDecisions, ShouldEqual, 0)

		position = slices.Collect(market.Desk.Positions())[0]
		stopped := position.Holding.Stoploss.Status == types.TRIGGERED &&
			position.Holding.Stoploss.TriggerReason != ""
		So(stopped, ShouldBeTrue)
		So(market.Desk.OpenPositions(), ShouldEqual, 0)
		So(position.Status, ShouldEqual, types.CLOSED)
		So(position.Holding.Status, ShouldEqual, types.CLOSED)
		So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
		So(position.Holding.ExitAt, ShouldNotBeNil)
	}))
}

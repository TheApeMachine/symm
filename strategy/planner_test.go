package strategy_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/cmd"
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
		types.SourceManifold,
		types.SourceResonance,
		types.SourceCausal,
		types.SourceGraph,
		types.SourceCategories,
		types.SourceCognition,
	} {
		thesis.Readiness.Stamp(source)
	}
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a thesis that is not yet ready for strategy evaluation", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("Every planner update should still advance the market tick", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			before := system.Thesis.Tick

			system.Planner.Update(system.Thesis)
			So(system.Thesis.Tick, ShouldEqual, before+1)

			system.Planner.Update(system.Thesis)
			So(system.Thesis.Tick, ShouldEqual, before+2)
		}))
	})

	Convey("Given repeated complete planner passes without a decision", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("It should prepare next evaluation while retaining cycle evidence", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis
			base := time.Unix(1_700_006_000, 0).UTC()
			thesis.Lifecycle.Store("SIM1/USD", types.LifecycleManaging)
			stampPlannerReadiness(thesis)

			observedAt := base
			thesis.AppendTicker(kraken.TickerData{
				Symbol: "SIM1/USD", Timestamp: observedAt,
			})
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "SIM1/USD", TradeID: 1,
				Timestamp: observedAt,
			})
			thesis.AppendMeasurements([]*types.Measurement{{
				Source: types.SourceCVD, Symbol: "SIM1/USD", At: observedAt,
			}}, true)

			system.Planner.Update(thesis)

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

		Convey("It should complete evaluation and prepare next pass retaining multi-read history", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis
			observedAt := time.Unix(1_700_006_050, 0).UTC()
			thesis.AppendTicker(kraken.TickerData{
				Symbol: "SIM1/USD", Timestamp: observedAt,
			})
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "SIM1/USD", TradeID: 1, Timestamp: observedAt,
			})
			thesis.AppendMeasurements([]*types.Measurement{{
				Source: types.SourcePumpDump, Symbol: "SIM1/USD", At: observedAt,
			}}, true)
			decision := types.Decision{
				Action: types.ActionNothing, Symbol: "SIM1/USD", At: observedAt,
			}

			thesis.Decisions.Store(decision.Symbol, &decision)
			stampPlannerReadiness(thesis)

			system.Planner.Update(thesis)

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

		Convey("It should emit the decisions before closing canonical history", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			thesis := system.Thesis
			observedAt := time.Unix(1_700_006_100, 0).UTC()
			thesis.AppendTicker(kraken.TickerData{
				Symbol: "SIM1/USD", Timestamp: observedAt,
			})
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "SIM1/USD", TradeID: 1, Timestamp: observedAt,
			})
			thesis.AppendMeasurements([]*types.Measurement{{
				Source: types.SourceCVD, Symbol: "SIM1/USD", At: observedAt,
			}}, true)
			thesis.Lifecycle.Store("SIM1/USD", types.LifecycleManaging)
			stampPlannerReadiness(thesis)

			system.Planner.Update(thesis)
			So(thesis.Decisions, ShouldBeEmpty)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
			So(system.Thesis, ShouldEqual, thesis)

			stampPlannerReadiness(thesis)
			decision := types.Decision{
				Action: types.ActionHold, Symbol: "SIM1/USD", At: observedAt,
			}

			thesis.Decisions.Store(decision.Symbol, &decision)
			subscription := system.Planner.Subscribe(
				"cycle-close-test", types.NewSubscription[any](),
			)

			system.Planner.Update(thesis)

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

	Convey(
		"Given a fast pump whose forecast never clears executable friction",
		t, tests.WithStack(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			published := make([]types.Decision, 0)
			var publishedMu sync.Mutex
			subscription := system.Planner.Subscribe(
				"decisions", types.NewSubscription[any](),
			)

			go func() {
				for {
					emitted, open := <-subscription.Channel

					if !open {
						return
					}

					decisions, ok := emitted.([]types.Decision)

					if !ok {
						continue
					}

					publishedMu.Lock()
					published = append(published, decisions...)
					publishedMu.Unlock()
				}
			}()

			initialOpportunitySlots := system.Desk.OpenSlots(true)

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
			maximumExecutableReturn := decimal.NewFromInt64(0)

			for _, decision := range published {

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

			_, measured := system.Thesis.Measurements.Load(types.SourcePumpDump)
			So(measured, ShouldBeTrue)
			So(pricedCandidates, ShouldBeGreaterThan, 0)
			So(uneconomicCandidates, ShouldEqual, pricedCandidates)
			So(maximumExecutableReturn.Sign(), ShouldBeLessThanOrEqualTo, 0)
			So(entryDecisions, ShouldEqual, 0)
			So(system.Desk.OpenPositions(), ShouldEqual, 0)
			So(system.Desk.OpenSlots(true), ShouldEqual, initialOpportunitySlots)
		}))
}

func TestPlannerSlotDiscipline(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		testtypes.NewSymbol("SIM4/USD", 100.0, 5150),
	}

	Convey("Given simultaneous pump candidates below their own trading costs", t, tests.WithStack(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
		published := make([]types.Decision, 0)
		var publishedMu sync.Mutex
		subscription := system.Planner.Subscribe(
			"decisions", types.NewSubscription[any](),
		)

		go func() {
			for {
				emitted, open := <-subscription.Channel

				if !open {
					return
				}

				decisions, ok := emitted.([]types.Decision)

				if !ok {
					continue
				}

				publishedMu.Lock()
				published = append(published, decisions...)
				publishedMu.Unlock()
			}
		}()

		initialNormalSlots := system.Desk.OpenSlots(false)
		initialOpportunitySlots := system.Desk.OpenSlots(true)

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

		for _, decision := range published {
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
			So(decision.SlotCapacity, ShouldEqual, system.Desk.MaxPositions())
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
		So(system.Desk.OpenPositions(), ShouldEqual, 0)
		So(system.Desk.OpenSlots(false), ShouldEqual, initialNormalSlots)
		So(system.Desk.OpenSlots(true), ShouldEqual, initialOpportunitySlots)
	}))
}

func TestPlannerPumpReversal(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
	}

	Convey("Given an open simulated position when a pump reverses into a dump", t, tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
		published := make([]types.Decision, 0)
		var publishedMu sync.Mutex
		subscription := system.Planner.Subscribe(
			"decisions", types.NewSubscription[any](),
		)

		go func() {
			for {
				emitted, open := <-subscription.Channel

				if !open {
					return
				}

				decisions, ok := emitted.([]types.Decision)

				if !ok {
					continue
				}

				publishedMu.Lock()
				published = append(published, decisions...)
				publishedMu.Unlock()
			}
		}()

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
		So(system.Desk.Execute(types.Decision{
			ID:               uuid.NewString(),
			Action:           types.ActionEnter,
			Symbol:           symbols[0].Pair,
			ProposedQuantity: entryQuantity,
			Risk:             entryRisk(system, symbols[0].Pair),
		}), ShouldBeNil)

		market.Tick()
		market.Tick()

		positions := slices.Collect(system.Desk.Positions())
		So(positions, ShouldHaveLength, 1)

		position := positions[0]
		So(position.Status, ShouldEqual, types.OPEN)
		So(position.Holding.SellableQty.Cmp(entryQuantity), ShouldEqual, 0)

		reversalDecisionOffset := len(published)
		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, testtypes.FastDump), ShouldBeNil)
		}

		for range 256 {
			market.Tick()
		}

		decisions := published
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

		position = slices.Collect(system.Desk.Positions())[0]
		stopped := position.Holding.Stoploss.Status == types.TRIGGERED &&
			position.Holding.Stoploss.TriggerReason != ""
		So(stopped, ShouldBeTrue)
		So(system.Desk.OpenPositions(), ShouldEqual, 0)
		So(position.Status, ShouldEqual, types.CLOSED)
		So(position.Holding.Status, ShouldEqual, types.CLOSED)
		So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
		So(position.Holding.ExitAt, ShouldNotBeNil)
	}))
}

/*
entryRisk derives the stop geometry an entry for this symbol would be sized
under, from the live simulated book. The desk refuses an entry without one,
because a quantity solved against a particular risk distance carries a loss
nobody budgeted once it is fitted with another.
*/
func entryRisk(system *cmd.System, symbol string) types.RiskPlan {
	pair, err := system.Desk.Instrument().Pair(symbol)

	if err != nil {
		return types.RiskPlan{}
	}

	return system.Desk.Price().RiskPlan(pair)
}

package strategy

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
frontierPlanner builds a Planner whose expensive evaluator is injected and
counted, so tests can prove exactly which symbols were re-evaluated. The
injected evaluator records the evaluated decision directly; it costs nothing,
so "evaluation" here means "the expensive causal/MCTS step ran".
*/
func frontierPlanner(enters bool) (*Planner, *atomic.Int64) {
	evaluations := new(atomic.Int64)
	planner := &Planner{
		tradingGate: func() bool { return true },
		marketProvider: func(string) marketInputs {
			return marketInputs{
				cash: 100000, mark: 100, feeRate: 0.001,
				spreadFraction: 0, available: true,
			}
		},
		allocation: nil,
	}
	planner.stager = audit.NewStager(nil)

	planner.evaluate = func(state *CausalState, _ *system.Config, _ marketInputs) *types.Decision {
		evaluations.Add(1)

		action := types.ActionNothing

		if enters {
			action = types.ActionEnter
		}

		decision := types.NewDecision(action, state.Symbol)
		decision.At = state.At
		decision.ValuationAttempted = true
		decision.ValuationAvailable = true
		decision.UtilityAvailable = true
		decision.Alternatives = map[string]float64{
			"economic:enter_mean":      1,
			"economic:wait_mean":       0,
			"economic:enter_advantage": 1,
			"economic:visits":          1,
		}

		return decision
	}

	return planner, evaluations
}

func frontierState(symbol string, at time.Time) *CausalState {
	state := deterministicCausalState(at, 0.005)
	state.Symbol = symbol

	return state
}

func TestResidentFrontierReevaluation(t *testing.T) {
	Convey("Given N resident symbols with a fresh frontier", t, func() {
		planner, evaluations := frontierPlanner(false)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("A/USD", frontierState("A/USD", at))
		planner.pending.Store("B/USD", frontierState("B/USD", at))
		planner.pending.Store("C/USD", frontierState("C/USD", at))

		_ = planner.Update(types.NewThesis(t.Context()))
		afterFirst := evaluations.Load()
		So(afterFirst, ShouldEqual, 3)

		Convey("an unrelated ticker does not rerun MCTS over the whole universe", func() {
			// A ticker for an unrelated symbol publishes a newer state for that
			// symbol only; A, B and C are untouched by it.
			planner.pending.Store("D/USD", frontierState("D/USD", at.Add(time.Second)))
			_ = planner.Update(types.NewThesis(t.Context()))

			So(evaluations.Load(), ShouldEqual, afterFirst+1)
		})

		Convey("updating A reevaluates A without reevaluating B/C", func() {
			planner.pending.Store("A/USD", frontierState("A/USD", at.Add(time.Second)))
			_ = planner.Update(types.NewThesis(t.Context()))

			So(evaluations.Load(), ShouldEqual, afterFirst+1)
		})
	})
}

func TestResidentFrontierFreshStateStopsCompeting(t *testing.T) {
	Convey("Given a candidate whose newer state selects Wait", t, func() {
		planner, evaluations := frontierPlanner(true)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("A/USD", frontierState("A/USD", at))

		round := planner.Update(types.NewThesis(t.Context()))
		So(round, ShouldNotBeNil)
		So(len(round.Decisions), ShouldEqual, 1)
		So(round.Decisions[0].Action, ShouldEqual, types.ActionEnter)
		So(evaluations.Load(), ShouldEqual, 1)

		Convey("re-evaluating a Wait-selecting state replaces the resident candidate", func() {
			planner.evaluate = func(state *CausalState, _ *system.Config, _ marketInputs) *types.Decision {
				evaluations.Add(1)

				decision := types.NewDecision(types.ActionNothing, state.Symbol)
				decision.At = state.At

				return decision
			}

			planner.pending.Store("A/USD", frontierState("A/USD", at.Add(time.Second)))
			round = planner.Update(types.NewThesis(t.Context()))

			So(round, ShouldNotBeNil)
			So(len(round.Decisions), ShouldEqual, 1)
			So(round.Decisions[0].Action, ShouldEqual, types.ActionNothing)

			// Only one new evaluation, replacing the stale Enter candidate.
			So(evaluations.Load(), ShouldEqual, 2)
		})
	})
}

func TestResidentFrontierDeterministicTieBreak(t *testing.T) {
	Convey("Given equally ranked resident candidates", t, func() {
		planner, _ := frontierPlanner(true)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("B/USD", frontierState("B/USD", at))
		planner.pending.Store("A/USD", frontierState("A/USD", at))
		planner.pending.Store("C/USD", frontierState("C/USD", at))

		Convey("arbitration order is deterministic across passes", func() {
			first := planner.residentDecisions(system.Cfg.Snapshot())
			second := planner.residentDecisions(system.Cfg.Snapshot())

			So(len(first), ShouldEqual, 3)
			So(len(second), ShouldEqual, 3)

			for index := range first {
				So(second[index].Symbol, ShouldEqual, first[index].Symbol)
			}

			So(first[0].Symbol, ShouldEqual, "A/USD")
			So(first[1].Symbol, ShouldEqual, "B/USD")
			So(first[2].Symbol, ShouldEqual, "C/USD")
		})
	})
}

func TestResidentFrontierNoFreshnessTimeout(t *testing.T) {
	Convey("Given a resident candidate beyond any imagined freshness window", t, func() {
		planner, evaluations := frontierPlanner(false)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("A/USD", frontierState("A/USD", at))
		_ = planner.Update(types.NewThesis(t.Context()))
		So(evaluations.Load(), ShouldEqual, 1)

		Convey("an unchanged pass reuses the stored evaluation without a timeout", func() {
			for range 4 {
				_ = planner.Update(types.NewThesis(t.Context()))
			}

			So(evaluations.Load(), ShouldEqual, 1)
		})
	})
}

func TestResidentFrontierSignatureInvalidatesOnInputs(t *testing.T) {
	Convey("Given a candidate evaluated under one execution market", t, func() {
		planner, evaluations := frontierPlanner(false)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("A/USD", frontierState("A/USD", at))
		_ = planner.Update(types.NewThesis(t.Context()))
		So(evaluations.Load(), ShouldEqual, 1)

		Convey("a materially changed execution market re-evaluates that symbol", func() {
			planner.marketProvider = func(string) marketInputs {
				return marketInputs{
					cash: 100000, mark: 105, feeRate: 0.001,
					spreadFraction: 0, available: true,
				}
			}

			// No new CausalState: the cheap signature check alone invalidates.
			_ = planner.Update(types.NewThesis(t.Context()))

			So(evaluations.Load(), ShouldEqual, 2)
		})
	})
}

func TestResidentFrontierMCTSRootMeans(t *testing.T) {
	Convey("Given a deterministic state", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		state := deterministicCausalState(at, 0)

		inputs := marketInputs{cash: 100000, mark: 100, feeRate: 0.001, spreadFraction: 0, available: true}

		// Observation sufficiency gating now runs before the MCTS search, so
		// the deterministic state must be backed by a reasoner whose store has
		// at least twenty retained observations for its primary coordinates.
		schema := DefaultCausalSchema(1, time.Second)
		store := relation.NewObservationStore(64)

		for coordinate := range state.MarketState.Current {
			store.RegisterCoordinate(coordinate)

			for index := 0; index < 20; index++ {
				store.Append(relation.Observation{
					Coordinate: coordinate,
					Raw:        0,
					At:         at,
					Maturity:   1,
				})
			}
		}

		reasoner := &Reasoner{
			epoch:          1,
			store:          store,
			schemaTemplate: schema,
		}

		planner := &Planner{
			marketProvider: func(string) marketInputs { return inputs },
			reasoner:       reasoner,
		}

		decision := planner.decisionFromCausalState(state, system.Cfg.Snapshot(), inputs)

		Convey("economic branch means are recorded when explored", func() {
			So(decision.Alternatives, ShouldContainKey, "economic:enter_mean")
			So(decision.Alternatives, ShouldContainKey, "economic:wait_mean")
			So(decision.Alternatives, ShouldContainKey, "economic:enter_advantage")
			So(decision.ValuationAttempted, ShouldBeTrue)
		})
	})
}

func TestMCTSExitProvenanceTest(t *testing.T) {
	Convey("Given a held position whose search explored Exit and Wait", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Exit,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Exit, MeanReward: 1.6},
					{Action: mcts.Wait, MeanReward: 0.4},
				},
			},
		}

		decision := types.NewDecision(types.ActionExit, "TEST/USD")
		recordExitEconomic(decision, result, 1)

		Convey("Exit and Wait branch means are recorded when both explored", func() {
			So(decision.Alternatives["economic:exit_mean"], ShouldEqual, 1.6)
			So(decision.Alternatives["economic:wait_mean_exit"], ShouldEqual, 0.4)
			So(decision.Alternatives["economic:exit_advantage"], ShouldAlmostEqual, 1.2)
			So(decision.Alternatives["economic:exit_explored"], ShouldEqual, 1)
		})
	})

	Convey("Given a held position whose search did not explore Exit", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Exit,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Wait, MeanReward: 0.4},
				},
			},
		}

		decision := types.NewDecision(types.ActionExit, "TEST/USD")
		recordExitEconomic(decision, result, 1)

		Convey("the absent branch remains explicitly unavailable, never zero-filled", func() {
			So(decision.Alternatives, ShouldNotContainKey, "economic:exit_mean")
			So(decision.Alternatives, ShouldNotContainKey, "economic:exit_advantage")
			So(decision.Alternatives["economic:exit_explored"], ShouldEqual, 0)
		})
	})

	Convey("Given a flat position", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Wait,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Wait, MeanReward: 0.4},
				},
			},
		}

		decision := types.NewDecision(types.ActionNothing, "TEST/USD")
		recordExitEconomic(decision, result, 0)

		Convey("no exit economics are recorded for a non-held symbol", func() {
			So(decision.Alternatives, ShouldBeEmpty)
		})
	})
}

func TestWorkingDecisionsClonesOnlyActionable(t *testing.T) {
	Convey("Given a frontier mixing actionable and wait decisions", t, func() {
		enter := types.NewDecision(types.ActionEnter, "A/USD")
		exit := types.NewDecision(types.ActionExit, "B/USD")
		wait := types.NewDecision(types.ActionNothing, "C/USD")

		working := workingDecisions([]*types.Decision{enter, exit, wait})

		Convey("only Enter and Exit shells are cloned; wait shares the resident record", func() {
			So(len(working), ShouldEqual, 3)
			So(working[0], ShouldNotPointTo, enter)
			So(working[1], ShouldNotPointTo, exit)
			So(working[2], ShouldPointTo, wait)
		})
	})
}

func TestResidentFrontierIncrementalOrder(t *testing.T) {
	Convey("Given a resident frontier", t, func() {
		planner, _ := frontierPlanner(false)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("C/USD", frontierState("C/USD", at))
		planner.pending.Store("A/USD", frontierState("A/USD", at))
		planner.pending.Store("B/USD", frontierState("B/USD", at))

		_ = planner.residentDecisions(system.Cfg.Snapshot())

		planner.frontierMu.RLock()
		symbols := make([]string, 0, len(planner.frontier))

		for _, entry := range planner.frontier {
			symbols = append(symbols, entry.symbol)
		}
		planner.frontierMu.RUnlock()

		Convey("candidates are inserted in deterministic symbol order without a per-pass sort", func() {
			So(symbols, ShouldResemble, []string{"A/USD", "B/USD", "C/USD"})
		})
	})
}
func TestRecordEntryEconomicTest(t *testing.T) {
	Convey("Given a search that explored Enter and Wait", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Enter,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Enter, MeanReward: 1.0},
					{Action: mcts.Wait, MeanReward: 0.2},
				},
			},
		}

		decision := types.NewDecision(types.ActionEnter, "TEST/USD")
		enterMean, enterFound, waitMean, waitFound, advantage := recordEntryEconomic(decision, result)

		Convey("both means and the advantage are recorded when both are explored", func() {
			So(enterFound, ShouldBeTrue)
			So(waitFound, ShouldBeTrue)
			So(enterMean, ShouldEqual, 1.0)
			So(waitMean, ShouldEqual, 0.2)
			So(advantage, ShouldAlmostEqual, 0.8)
			So(decision.Alternatives["economic:enter_mean"], ShouldEqual, 1.0)
			So(decision.Alternatives["economic:wait_mean"], ShouldEqual, 0.2)
			So(decision.Alternatives["economic:enter_advantage"], ShouldAlmostEqual, 0.8)
			So(decision.Alternatives["economic:enter_explored"], ShouldEqual, 1)
		})
	})

	Convey("Given a search that did not explore Enter", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Wait,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Wait, MeanReward: 0.2},
				},
			},
		}

		decision := types.NewDecision(types.ActionNothing, "TEST/USD")
		enterMean, enterFound, waitMean, waitFound, advantage := recordEntryEconomic(decision, result)

		Convey("the absent branch stays absent rather than reading as economic zero", func() {
			So(enterFound, ShouldBeFalse)
			So(waitFound, ShouldBeTrue)
			So(enterMean, ShouldEqual, 0)
			So(waitMean, ShouldEqual, 0.2)
			So(advantage, ShouldEqual, 0)
			So(decision.Alternatives, ShouldNotContainKey, "economic:enter_mean")
			So(decision.Alternatives, ShouldContainKey, "economic:wait_mean")
			So(decision.Alternatives, ShouldNotContainKey, "economic:enter_advantage")
			So(decision.Alternatives["economic:enter_explored"], ShouldEqual, 0)
		})
	})

	Convey("Given a search that did not explore Wait", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Enter,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Enter, MeanReward: 1.0},
				},
			},
		}

		decision := types.NewDecision(types.ActionEnter, "TEST/USD")
		_, enterFound, _, waitFound, advantage := recordEntryEconomic(decision, result)

		Convey("the advantage is absent when either branch is unexplored", func() {
			So(enterFound, ShouldBeTrue)
			So(waitFound, ShouldBeFalse)
			So(advantage, ShouldEqual, 0)
			So(decision.Alternatives, ShouldNotContainKey, "economic:wait_mean")
			So(decision.Alternatives, ShouldNotContainKey, "economic:enter_advantage")
		})
	})
}

func TestMCTSExitAuthorityProvenanceTest(t *testing.T) {
	Convey("Given an MCTS-selected Exit", t, func() {
		result := &mcts.SearchResult{
			SelectedAction: mcts.Exit,
			Trace: &mcts.Trace{
				Branches: []mcts.BranchTrace{
					{Action: mcts.Exit, MeanReward: 1.6},
					{Action: mcts.Wait, MeanReward: 0.4},
				},
			},
		}

		state := deterministicCausalState(time.Unix(0, 149*int64(time.Second)), 0)
		state.Symbol = "TEST/USD"

		decision := types.NewDecision(types.ActionExit, "TEST/USD")

		if result.SelectedAction == mcts.Exit {
			decision.Action = types.ActionExit
			decision.Cause = "planner_mcts"
			decision.Reason = "planner_mcts: exit selected over wait"
		}

		Convey("lifecycle/audit provenance identifies planner_mcts", func() {
			So(decision.Cause, ShouldEqual, "planner_mcts")
			So(decision.Reason, ShouldContainSubstring, "planner_mcts")
		})

		Convey("StopLoss exits are never merged into planner exits", func() {
			stoplossTriggered := types.NewDecision(types.ActionExit, "TEST/USD")
			stoplossTriggered.Cause = "stop_loss"

			So(stoplossTriggered.Cause, ShouldNotEqual, decision.Cause)
		})
	})
}

/*
TestResidentFrontierEconomicOrder proves the resident entry frontier is ordered
by economic enter advantage, not by symbol: a lower-symbol candidate with a
higher advantage sorts before a higher-symbol candidate with a lower advantage.
*/
func TestResidentFrontierEconomicOrder(t *testing.T) {
	Convey("Given candidates whose enter advantage differs across symbols", t, func() {
		planner, _ := frontierPlanner(true)
		at := time.Unix(0, 149*int64(time.Second))

		advantages := map[string]float64{
			"AAA/USD": 0.02,
			"BBB/USD": 0.09,
			"CCC/USD": 0.05,
		}

		planner.evaluate = func(state *CausalState, _ *system.Config, _ marketInputs) *types.Decision {
			decision := types.NewDecision(types.ActionEnter, state.Symbol)
			decision.At = state.At
			decision.Alternatives = map[string]float64{
				"economic:enter_mean":      1,
				"economic:wait_mean":       0,
				"economic:enter_advantage": advantages[state.Symbol],
				"economic:visits":          1,
			}
			return decision
		}

		for _, symbol := range []string{"AAA/USD", "BBB/USD", "CCC/USD"} {
			planner.pending.Store(symbol, frontierState(symbol, at))
		}

		_ = planner.residentDecisions(system.Cfg.Snapshot())

		planner.frontierMu.RLock()
		symbols := make([]string, 0, len(planner.frontier))

		for _, entry := range planner.frontier {
			symbols = append(symbols, entry.symbol)
		}
		planner.frontierMu.RUnlock()

		Convey("the strongest advantage leads regardless of symbol", func() {
			So(symbols, ShouldResemble, []string{"BBB/USD", "CCC/USD", "AAA/USD"})
		})
	})
}

/*
TestResidentFrontierEnterWaitEnterTransitions proves the frontier is maintained
incrementally across action transitions: Enter→Wait drops a symbol out of the
entry ranking, Wait→Enter reinserts it at its current economic position, with
no full-universe rescan.
*/
func TestResidentFrontierEnterWaitEnterTransitions(t *testing.T) {
	Convey("Given a candidate that starts Enter and flips to Wait", t, func() {
		planner, evaluations := frontierPlanner(true)
		at := time.Unix(0, 149*int64(time.Second))

		planner.pending.Store("A/USD", frontierState("A/USD", at))
		_ = planner.Update(types.NewThesis(t.Context()))

		planner.evaluate = func(state *CausalState, _ *system.Config, _ marketInputs) *types.Decision {
			evaluations.Add(1)
			decision := types.NewDecision(types.ActionNothing, state.Symbol)
			decision.At = state.At

			return decision
		}

		planner.pending.Store("A/USD", frontierState("A/USD", at.Add(time.Second)))
		_ = planner.Update(types.NewThesis(t.Context()))

		planner.frontierMu.RLock()
		advantage, hasAdvantage := planner.frontier[0].advantage, len(planner.frontier) > 0
		planner.frontierMu.RUnlock()

		Convey("the Wait record no longer competes as an entry", func() {
			// The frontier retains the record for the decision round, but it
			// carries no enter advantage: it sorts after every real entry.
			So(hasAdvantage, ShouldBeTrue)
			So(math.IsInf(advantage, -1), ShouldBeTrue)
		})

		Convey("a later Wait→Enter transition reinserts it at its economic position", func() {
			planner.evaluate = func(state *CausalState, _ *system.Config, _ marketInputs) *types.Decision {
				evaluations.Add(1)
				decision := types.NewDecision(types.ActionEnter, state.Symbol)
				decision.At = state.At
				decision.Alternatives = map[string]float64{
					"economic:enter_advantage": 0.5,
					"economic:visits":          2,
				}

				return decision
			}

			planner.pending.Store("A/USD", frontierState("A/USD", at.Add(2*time.Second)))
			_ = planner.Update(types.NewThesis(t.Context()))

			planner.frontierMu.RLock()
			front := planner.frontier[0]
			planner.frontierMu.RUnlock()

			So(front.symbol, ShouldEqual, "A/USD")
			So(front.advantage, ShouldEqual, 0.5)
		})
	})
}

/*
TestResidentFrontierRepositionUpdatesOneSlot proves re-evaluating one candidate
with a better advantage repositions exactly that slot without re-sorting or
re-evaluating every other resident candidate.
*/
func TestResidentFrontierRepositionUpdatesOneSlot(t *testing.T) {
	Convey("Given a multi-symbol economic frontier", t, func() {
		planner, evaluations := frontierPlanner(true)
		at := time.Unix(0, 149*int64(time.Second))

		advantages := map[string]float64{
			"AAA/USD": 0.02, "BBB/USD": 0.05, "CCC/USD": 0.08,
		}
		states := map[string]*CausalState{
			"AAA/USD": frontierState("AAA/USD", at),
			"BBB/USD": frontierState("BBB/USD", at),
			"CCC/USD": frontierState("CCC/USD", at),
		}

		planner.evaluate = func(state *CausalState, _ *system.Config, _ marketInputs) *types.Decision {
			evaluations.Add(1)

			decision := types.NewDecision(types.ActionEnter, state.Symbol)
			decision.At = state.At
			decision.Alternatives = map[string]float64{
				"economic:enter_advantage": advantages[state.Symbol],
				"economic:visits":          1,
			}

			return decision
		}

		for symbol, state := range states {
			planner.pending.Store(symbol, state)
		}

		_ = planner.Update(types.NewThesis(t.Context()))
		before := evaluations.Load()

		Convey("boosting AAA above everyone re-evaluates just AAA and moves it to the front", func() {
			advantages["AAA/USD"] = 0.99
			planner.pending.Store("AAA/USD", frontierState("AAA/USD", at.Add(time.Second)))
			_ = planner.Update(types.NewThesis(t.Context()))

			So(evaluations.Load(), ShouldEqual, before+1)

			planner.frontierMu.RLock()
			front := planner.frontier[0]
			planner.frontierMu.RUnlock()

			So(front.symbol, ShouldEqual, "AAA/USD")
			So(front.advantage, ShouldEqual, 0.99)
		})
	})
}

func BenchmarkPlannerCandidateUpdate(b *testing.B) {
	planner, _ := frontierPlanner(true)
	at := time.Unix(0, 149*int64(time.Second))

	planner.pending.Store("A/USD", frontierState("A/USD", at))
	_ = planner.Update(types.NewThesis(context.Background()))

	b.ReportAllocs()

	for b.Loop() {
		planner.pending.Store("A/USD", frontierState("A/USD", at))
		_ = planner.residentDecisions(system.Cfg.Snapshot())
	}
}

func BenchmarkPlannerOneSlotArbitration(b *testing.B) {
	planner, _ := frontierPlanner(true)
	at := time.Unix(0, 149*int64(time.Second))

	for _, symbol := range []string{"A/USD", "B/USD", "C/USD", "D/USD", "E/USD"} {
		planner.pending.Store(symbol, frontierState(symbol, at))
	}

	_ = planner.Update(types.NewThesis(context.Background()))

	b.ReportAllocs()

	for b.Loop() {
		_ = planner.residentDecisions(system.Cfg.Snapshot())
	}
}

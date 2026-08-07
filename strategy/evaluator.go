package strategy

import (
	"sync"

	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

type Evaluator struct {
	mctsEngine *mcts.CausalMCTS
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	passage    *types.PassageModel
	recorder   *audit.Recorder
}

func NewEvaluator(
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	recorder *audit.Recorder,
) Evaluator {
	engine := NewCausalEngineAdapter()

	mctsSearch := mcts.NewCausalMCTS(
		engine,
		1.414, 0.5,
		mctsMinimumCausalRows, 2, 3,
		[]int{0, 1}, []int{0, 1, 2},
		false,
	)

	return Evaluator{
		mctsEngine: mctsSearch,
		desk:       desk,
		price:      price,
		balance:    balance,
		passage:    types.NewPassageModel(),
		recorder:   recorder,
	}
}

/*
record stores one verdict under the symbol it was reached about, which is the
key every later stage reads a symbol's decision back by. One evaluation reaches
one verdict per symbol, so a second store for the same symbol replaces the first
rather than leaving the pass holding two answers to the same question.
*/
func (evaluator Evaluator) record(thesis *types.Thesis, decision types.Decision) {
	thesis.Decisions.Store(decision.Symbol, &decision)
}

func (evaluator Evaluator) EvaluateOpportunities(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	if thesis.Cognition == nil {
		thesis.Cognition = &sync.Map{}
	}

	if thesis.Causal == nil {
		thesis.Causal = &sync.Map{}
	}

	if thesis.Graphs == nil {
		thesis.Graphs = &sync.Map{}
	}

	if thesis.Lifecycle == nil {
		thesis.Lifecycle = &sync.Map{}
	}

	for _, symbol := range thesis.MarketSymbols() {
		rootState := StrategyState{
			Symbol:    symbol,
			Reward:    0.0,
			Step:      0,
			IsHolding: false,
		}

		trace := &types.DecisionTrace{
			Utility: types.DecisionUtilityTrace{},
			MCTS: rootState.Trace(
				evaluator.mctsEngine.MinRows,
				mctsSearchIterations,
				1,
			),
		}

		if trace.MCTS.Searchable {
			continue
		}

		var recommendedAction float64
		var mctsErr error

		searchable := trace.MCTS.Searchable

		if searchable {
			trace.MCTS.Attempted = true

			recommendedAction, mctsErr = evaluator.mctsEngine.Search(
				rootState,
				mctsSearchIterations,
				[][]float64{{
					rootState.Energy,
					rootState.Surprise,
					rootState.Treatment,
				}},
			)

			if mctsErr != nil {
				trace.MCTS.Error = mctsErr.Error()
			}

			if mctsErr == nil {
				trace.MCTS.RecommendedAction = strategyAction(recommendedAction)
			}
		}

		utility := 1

		if utility <= 0 {
			continue
		}

		if searchable && (mctsErr != nil || recommendedAction != ActionEnter) {
			continue
		}
	}

	thesis.Stamp(types.SourceEvaluator)
}

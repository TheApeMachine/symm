package strategy

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Planner struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	ui         chan []byte
	recorder   *audit.Recorder
	mctsEngine *mcts.CausalMCTS
	allocation *Allocation
	desk       *broker.Desk
}

func NewPlanner(
	ctx context.Context,
	uiHub chan []byte,
	thesis *types.Thesis,
	recorder *audit.Recorder,
	desk *broker.Desk,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	planner := &Planner{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.READY,
		ui:       uiHub,
		recorder: recorder,
		mctsEngine: mcts.NewCausalMCTS(
			mcts.DefaultCausalEngine{},
			math.Sqrt2,
			1,
			len(mcts.GraphFeatureColumns)+1,
			mcts.GraphTreatmentColumn,
			mcts.GraphTargetColumn,
			mcts.GraphControlColumns,
			mcts.GraphFeatureColumns,
			false,
		),
		allocation: NewAllocation(ctx, desk),
		desk:       desk,
	}

	return planner
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) Update(thesis *types.Thesis) error {
	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return fmt.Errorf("planner: planner configuration required")
	}

	decisions := make([]types.Decision, 0)
	var err error

	thesis.Symbols.Range(func(key, value any) bool {
		symbol := key.(string)
		symbolState := value.(*types.Symbol)
		symbolState.Decisions.Clear()

		stored, found := symbolState.Graphs.Load("market_graph")

		if !found {
			return true
		}

		graph := stored.(*logicgraph.Graph)

		if graph.Forecast == nil || !graph.Forecast.Ready ||
			graph.Forecast.Value <= 0 {
			return true
		}

		state, stateErr := mcts.NewGraphState(graph)

		if stateErr != nil {
			err = fmt.Errorf("planner: graph state for %s: %w", symbol, stateErr)
			return false
		}

		history := state.History()

		planner.mctsEngine.C = config.Planner.ExplorationConstant
		planner.mctsEngine.CausalAlpha = config.Planner.CausalAlpha

		root, action, searchErr := planner.mctsEngine.Search(
			state, config.Planner.MCTSIterations, history,
		)

		if searchErr != nil {
			err = fmt.Errorf("planner: graph search for %s: %w", symbol, searchErr)
			return false
		}

		decision := types.NewDecision(types.ActionNothing, symbol)
		decision.At = graph.At
		decision.Forecast = graph.Forecast
		decision.Confidence = config.Planner.MinimumConfidence
		decision.Alternatives = make(map[string]float64)

		for _, branch := range root.Children {
			utility := branch.TotalReward / float64(branch.Visits)
			decision.Alternatives[fmt.Sprintf("%g", branch.Action)] = utility

			if branch.Action == action {
				decision.Utility = utility
			}
		}

		if decision.Utility > 0 {
			decision.Action = types.ActionEnter
			decision.Cause = "opportunity_entry"
		}

		symbolState.Decisions.Store(symbol, decision)
		decisions = append(decisions, *decision)
		return true
	})

	if err != nil {
		return err
	}

	if planner.allocation != nil {
		if err = planner.allocation.Calculate(thesis); err != nil {
			return err
		}
	}

	utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
		"evaluated", true,
		"outcome", "decisions",
		"decisions", decisions,
	)))

	return nil
}

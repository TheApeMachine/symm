package strategy

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"gonum.org/v1/gonum/stat/distuv"
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
	utils.Publish(planner.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourcePlanner), "running",
	)))

	defer utils.Publish(planner.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourcePlanner), "done",
	)))

	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return fmt.Errorf("planner: planner configuration required")
	}

	createdDecisions := make([]*types.Decision, 0)
	var err error

	thesis.Symbols.Range(func(key, value any) bool {
		symbol := key.(string)
		symbolState := value.(*types.Symbol)

		stored, found := symbolState.Graphs.Load("market_graph")

		if !found {
			return true
		}

		graph := stored.(*logicgraph.Graph)

		if !graph.ReadyForSearch(config.Planner.MinimumSkill) {
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
		confidence, confidenceErr := forecastDirectionConfidence(graph.Forecast)

		if confidenceErr != nil {
			err = fmt.Errorf("planner: forecast confidence for %s: %w", symbol, confidenceErr)
			return false
		}

		decision.Confidence = confidence
		decision.Alternatives = make(map[string]float64)
		decision.Trace = decisionTrace(
			graph,
			root,
			action,
			config.Planner.MCTSIterations,
		)

		for _, branch := range root.Children {
			utility := branch.TotalReward / float64(branch.Visits)
			decision.Alternatives[graphActionLabel(graph.Roots(), branch.Action)] = utility

			if branch.Action == action {
				decision.Utility = utility
			}
		}

		if graph.Forecast.Value > 0 &&
			decision.Utility > config.Planner.MinimumUtility &&
			decision.Confidence >= config.Planner.MinimumConfidence {
			decision.Action = types.ActionEnter
			decision.Cause = "opportunity_entry"
		}

		if graph.Forecast.Value <= 0 {
			decision.Reason = "planner: forecast does not support entry"
		}

		if graph.Forecast.Value > 0 &&
			decision.Confidence < config.Planner.MinimumConfidence {
			decision.Reason = "planner: forecast confidence does not clear regulated entry threshold"
		}

		if decision.Action != types.ActionEnter && decision.Reason == "" {
			decision.Reason = "planner: utility does not clear regulated entry threshold"
		}

		createdDecisions = append(createdDecisions, decision)
		return true
	})

	if err != nil {
		return err
	}

	if len(createdDecisions) == 0 {
		return nil
	}

	if planner.allocation != nil {
		if err = planner.allocation.Calculate(createdDecisions); err != nil {
			return err
		}
	}

	for _, decision := range createdDecisions {
		symbol := thesis.Symbol(decision.Symbol)
		symbol.Decisions.Store(decision.Symbol, decision)
	}

	decisions := make([]types.Decision, 0, len(createdDecisions))
	actionable := false

	for _, decision := range createdDecisions {
		decisions = append(decisions, *decision)

		if decision.Action != types.ActionNothing {
			actionable = true
		}
	}

	if !actionable {
		utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
			"evaluated", false,
			"outcome", "accumulating",
			"decisions", decisions,
		)))

		return nil
	}

	if planner.desk != nil {
		if err = planner.desk.SaveThesis(thesis); err != nil {
			return fmt.Errorf("planner: checkpoint evaluated thesis: %w", err)
		}
	}

	if planner.desk != nil {
		for _, decision := range createdDecisions {
			if decision.Action != types.ActionEnter {
				continue
			}

			if err = planner.desk.Execute(*decision); err != nil {
				return fmt.Errorf("planner: execute %s: %w", decision.Symbol, err)
			}
		}
	}

	for _, decision := range createdDecisions {
		thesis.Symbol(decision.Symbol).Graphs.Store(
			"market_graph",
			logicgraph.NewGraph(thesis.At),
		)
	}

	utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
		"evaluated", true,
		"outcome", "decisions",
		"decisions", decisions,
	)))

	return nil
}

func forecastDirectionConfidence(forecast *learning.RLSOutput) (float64, error) {
	if forecast == nil || !forecast.Ready || forecast.Scale <= 0 ||
		forecast.DegreesOfFreedom <= 0 {
		return 0, fmt.Errorf("ready posterior predictive forecast required")
	}

	distribution := distuv.StudentsT{
		Mu:    forecast.Value,
		Sigma: forecast.Scale,
		Nu:    forecast.DegreesOfFreedom,
	}

	return 1 - distribution.CDF(0), nil
}

func decisionTrace(
	graph *logicgraph.Graph,
	root *mcts.Node,
	recommended float64,
	iterations int,
) *types.DecisionTrace {
	supports, contradicts := graphEvidenceMass(graph)
	branches := make([]types.DecisionMCTSBranch, 0, len(root.Children))
	roots := graph.Roots()

	for _, branch := range root.Children {
		branches = append(branches, types.DecisionMCTSBranch{
			Action:     graphActionLabel(roots, branch.Action),
			Visits:     branch.Visits,
			MeanReward: branch.TotalReward / float64(branch.Visits),
		})
	}

	slices.SortFunc(branches, func(left, right types.DecisionMCTSBranch) int {
		if left.Visits == right.Visits {
			return 0
		}

		if left.Visits > right.Visits {
			return -1
		}

		return 1
	})

	return &types.DecisionTrace{
		GraphSupports:    supports,
		GraphContradicts: contradicts,
		MCTS: types.DecisionMCTSTrace{
			Iterations:        iterations,
			Branches:          branches,
			RecommendedAction: graphActionLabel(roots, recommended),
		},
	}
}

func graphActionLabel(roots []string, action float64) string {
	index := int(action)

	if index >= 0 && index < len(roots) && action == float64(index) {
		return roots[index]
	}

	return fmt.Sprintf("root[%g]", action)
}

func graphEvidenceMass(graph *logicgraph.Graph) (float64, float64) {
	supports := 0.0
	contradicts := 0.0

	for _, edge := range graph.Edges {
		mass := edge.Weight * edge.Confidence

		if edge.Relation == logicgraph.RelationSupports {
			supports += mass
		}

		if edge.Relation == logicgraph.RelationContradicts {
			contradicts += mass
		}
	}

	return supports, contradicts
}

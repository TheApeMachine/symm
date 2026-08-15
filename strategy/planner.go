package strategy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
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

	candidateMu sync.Mutex
	candidates  map[string]*types.Decision

	ObserveModule func(string, time.Duration)
	ObserveHop    func(string, string, time.Duration)
	executeEntry  func(types.Decision) error
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
		candidates: make(map[string]*types.Decision),
	}

	return planner
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

/*
HasCapacity reports whether a new normal-slot entry can still be admitted.
A nil desk is treated as open capacity so focused planner tests stay intact.
*/
func (planner *Planner) HasCapacity() bool {
	if planner == nil || planner.desk == nil {
		return true
	}

	return planner.desk.OpenSlots(false) > 0
}

/*
Holding reports whether the desk already carries the named symbol.
*/
func (planner *Planner) Holding(symbol string) bool {
	if planner == nil || planner.desk == nil || symbol == "" {
		return false
	}

	return planner.desk.Holding(symbol) > 0
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) Update(thesis *types.Thesis) error {
	utils.PublishPriority(planner.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourcePlanner), "running",
	)))

	defer utils.PublishPriority(planner.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourcePlanner), "done",
	)))

	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return fmt.Errorf("planner: planner configuration required")
	}

	createdDecisions := make([]*types.Decision, 0)
	var err error
	plannerStarted := time.Now()
	lastSearchEnd := plannerStarted

	defer func() {
		if planner.ObserveModule != nil {
			planner.ObserveModule("planner", time.Since(plannerStarted))
		}
	}()

	thesis.Symbols.Range(func(key, value any) bool {
		symbol := key.(string)
		symbolState := value.(*types.Symbol)

		stored, found := symbolState.Graphs.Load("market_graph")

		if !found {
			return true
		}

		graph := stored.(*logicgraph.Graph)

		if !graph.ReadyForSearch() {
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

		searchStarted := time.Now()

		if planner.ObserveHop != nil {
			planner.ObserveHop("planner", "mcts", searchStarted.Sub(lastSearchEnd))
		}

		root, action, searchErr := planner.mctsEngine.Search(
			state, config.Planner.MCTSIterations, history,
		)
		lastSearchEnd = time.Now()

		if planner.ObserveModule != nil {
			planner.ObserveModule("mcts", lastSearchEnd.Sub(searchStarted))
		}

		if searchErr != nil {
			err = fmt.Errorf("planner: graph search for %s: %w", symbol, searchErr)
			return false
		}

		decision := types.NewDecision(types.ActionNothing, symbol)
		decision.At = graph.At
		decision.Forecast = graph.Forecast
		decision.ForecastHorizon = graph.ForecastHorizon
		decision.ForwardCurve = slices.Clone(graph.ForwardCurve)
		confidence, confidenceErr := forecastDirectionConfidence(graph.Forecast)

		if confidenceErr != nil {
			err = fmt.Errorf("planner: forecast confidence for %s: %w", symbol, confidenceErr)
			return false
		}

		decision.Confidence = confidence
		perspectiveReturn, perspectiveConfidence, perspectiveSources, perspectiveErr :=
			decisionPerspective(graph, confidence)

		if perspectiveErr != nil {
			err = fmt.Errorf("planner: decision perspective for %s: %w", symbol, perspectiveErr)
			return false
		}

		decision.PerspectiveReturn = perspectiveReturn
		decision.PerspectiveConfidence = perspectiveConfidence
		decision.PerspectiveSources = perspectiveSources
		decision.ExpectedReturn = decimal.NewFromFloat64(math.Expm1(perspectiveReturn))
		decision.AdmissionGraphThreshold = config.Planner.MinimumGraphScore
		decision.AdmissionUtilityThreshold = config.Planner.MinimumUtility
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
				decision.GraphScore = utility
			}
		}

		if decision.GraphScore > 0 &&
			decision.GraphScore >= config.Planner.MinimumGraphScore {
			decision.Action = types.ActionEnter
			decision.Cause = "opportunity_entry"
		}

		if decision.Action != types.ActionEnter {
			decision.Reason = "planner: graph perspective does not clear regulated admission boundary"
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

	for _, decision := range createdDecisions {
		planner.retainCandidate(decision)
	}

	retireDecisionGraphs(thesis, createdDecisions)

	createdDecisions = planner.candidateCopies()

	if len(createdDecisions) == 0 {
		return nil
	}

	if planner.allocation != nil {
		allocationStarted := time.Now()

		if planner.ObserveHop != nil {
			planner.ObserveHop("mcts", "allocation", allocationStarted.Sub(lastSearchEnd))
		}

		if err = planner.allocation.Calculate(createdDecisions); err != nil {
			return err
		}

		if planner.ObserveModule != nil {
			planner.ObserveModule("allocation", time.Since(allocationStarted))
		}

		lastSearchEnd = time.Now()
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

	if err = audit.Record(planner.recorder, decisions); err != nil {
		var saturated types.SaturatedError

		if !errors.As(err, &saturated) {
			errnie.Error(fmt.Errorf("planner: audit evaluated decisions: %w", err))
		}
	}

	if !actionable {
		// This graph was evaluated. Retaining it would cause the identical
		// structural search to be repeated on every subsequent ready tick.
		retireDecisionGraphs(thesis, createdDecisions)

		utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
			"evaluated", false,
			"outcome", "accumulating",
			"decisions", decisions,
		)))

		return nil
	}

	if planner.desk != nil {
		winners := make([]*types.Decision, 0, len(createdDecisions))

		for _, decision := range createdDecisions {
			if decision.Action != types.ActionEnter {
				continue
			}

			winners = append(winners, decision)
		}

		slices.SortFunc(winners, admissionOrder)

		for _, decision := range winners {
			executeStarted := time.Now()

			if planner.ObserveHop != nil {
				planner.ObserveHop("allocation", "desk", executeStarted.Sub(lastSearchEnd))
			}

			if planner.executeEntry != nil {
				err = planner.executeEntry(*decision)
			} else {
				err = planner.desk.Execute(*decision)
			}

			if err != nil {
				decision.Action = types.ActionNothing
				decision.Reason = "planner: entry is no longer executable: " + err.Error()

				if !errnie.IsNotAcceptable(err) {
					return fmt.Errorf("planner: execute %s: %w", decision.Symbol, err)
				}

				continue
			}

			planner.removeCandidate(decision.Symbol)

			lastSearchEnd = time.Now()
		}
	}

	retireDecisionGraphs(thesis, createdDecisions)

	decisions = decisions[:0]

	for _, decision := range createdDecisions {
		decisions = append(decisions, *decision)
	}

	utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
		"evaluated", true,
		"outcome", "decisions",
		"decisions", decisions,
	)))

	return nil
}

func (planner *Planner) removeCandidate(symbol string) {
	planner.candidateMu.Lock()
	delete(planner.candidates, symbol)
	planner.candidateMu.Unlock()
}

func (planner *Planner) retainCandidate(decision *types.Decision) {
	if planner == nil || decision == nil || decision.Symbol == "" {
		return
	}

	planner.candidateMu.Lock()
	defer planner.candidateMu.Unlock()

	if decision.Action != types.ActionEnter {
		// A newer structural evaluation no longer endorses entry.
		delete(planner.candidates, decision.Symbol)
		return
	}

	candidate := *decision

	// These belong to live execution admission, not the structural candidate.
	candidate.AvailableCapital = nil
	candidate.ProposedNotional = nil
	candidate.ProposedQuantity = nil
	candidate.EntryPrice = nil
	candidate.Mark = nil
	candidate.Stoploss = nil
	candidate.ExpectedFees = nil
	candidate.ExpectedSpread = nil
	candidate.ExpectedImpact = nil
	candidate.Utility = 0
	candidate.OpportunityMargin = 0

	planner.candidates[decision.Symbol] = &candidate
}

func (planner *Planner) candidateCopies() []*types.Decision {
	planner.candidateMu.Lock()
	defer planner.candidateMu.Unlock()

	decisions := make([]*types.Decision, 0, len(planner.candidates))

	for symbol, retained := range planner.candidates {
		if retained == nil {
			delete(planner.candidates, symbol)
			continue
		}

		if planner.Holding(symbol) {
			delete(planner.candidates, symbol)
			continue
		}

		candidate := *retained
		candidate.Action = types.ActionEnter
		candidate.Reason = ""
		candidate.Stoploss = nil

		decisions = append(decisions, &candidate)
	}

	return decisions
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
	visited := make(map[string]bool)
	queue := append([]string(nil), graph.Roots()...)

	for len(queue) > 0 {
		source := queue[0]
		queue = queue[1:]

		if visited[source] {
			continue
		}

		visited[source] = true

		for _, edge := range graph.Edges {
			if edge.From != source {
				continue
			}

			queue = append(queue, edge.To)
			mass := edge.Weight * edge.Confidence

			if edge.Relation == logicgraph.RelationSupports {
				supports += mass
			}

			if edge.Relation == logicgraph.RelationContradicts {
				contradicts += mass
			}
		}
	}

	return supports, contradicts
}

func retireDecisionGraphs(
	thesis *types.Thesis,
	decisions []*types.Decision,
) {
	if thesis == nil {
		return
	}

	for _, decision := range decisions {
		if decision == nil || decision.Symbol == "" {
			continue
		}

		symbol := thesis.Symbol(decision.Symbol)
		current, found := symbol.Graphs.Load("market_graph")

		if !found {
			continue
		}

		graph, valid := current.(*logicgraph.Graph)

		if !valid || graph == nil {
			continue
		}

		// CompareAndSwap makes the lifecycle transition explicit. It also avoids
		// replacing a newer graph should planner ownership later become async.
		symbol.Graphs.CompareAndSwap(
			"market_graph",
			graph,
			logicgraph.NewGraph(thesis.At),
		)
	}
}

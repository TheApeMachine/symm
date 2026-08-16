package strategy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
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
		perspective, perspectiveErr := graphPerspective(graph)

		if perspectiveErr != nil {
			err = fmt.Errorf("planner: decision perspective for %s: %w", symbol, perspectiveErr)
			return false
		}

		decision.ThesisScore = perspective.Score
		decision.ThesisConfidence = perspective.Confidence
		decision.ThesisSupport = perspective.Support
		decision.ThesisContradiction = perspective.Contradiction
		decision.ThesisConditions = perspective.Conditions
		decision.Direction = perspective.Direction
		decision.Confidence = perspective.Confidence
		decision.PerspectiveConfidence = perspective.Confidence
		decision.AdmissionGraphThreshold = config.Planner.MinimumGraphScore
		decision.OpportunityType = graphOpportunityType(graph)
		decision.TaskSkill = graph.TaskSkill
		decision.TaskSkillReady = graph.TaskSkillReady
		decision.PredictiveReady, decision.PredictiveStatus = predictiveReadiness(graph)
		decision.ReserveEligible, decision.ReserveReason = reserveQualification(
			decision.OpportunityType,
			decision.PredictiveReady,
			decision.ForecastHorizon,
		)
		decision.Opportunity = decision.ReserveEligible
		decision.Alternatives = make(map[string]float64)
		decision.Trace = decisionTrace(
			graph,
			root,
			action,
			config.Planner.MCTSIterations,
		)

		for _, branch := range root.Children {
			if branch.Visits <= 0 {
				continue
			}

			reward := branch.TotalReward / float64(branch.Visits)
			decision.Alternatives[graphActionLabel(graph.Roots(), branch.Action)] = reward

			if branch.Action == action {
				decision.GraphScore = reward
			}
		}

		switch {
		case decision.Direction <= 0 || decision.ThesisScore <= 0:
			decision.Reason = "planner: contradiction outweighs support for the long-opportunity thesis"
		case decision.ThesisScore < config.Planner.MinimumGraphScore:
			decision.Reason = "planner: structural thesis does not clear the regulated evidence boundary"
		case decision.GraphScore <= 0 ||
			decision.GraphScore < config.Planner.MinimumGraphScore:
			decision.Reason = "planner: causal graph search did not retain a supportive evidence path"
		case !decision.PredictiveReady:
			decision.Reason = "planner: predictive coder cannot yet support an entry: " +
				decision.PredictiveStatus
		default:
			decision.Action = types.ActionEnter
			decision.Cause = "structural_long_opportunity"

			if decision.OpportunityType != "" {
				decision.Cause = decision.OpportunityType
			}
		}

		createdDecisions = append(createdDecisions, decision)
		return true
	})

	if err != nil {
		return err
	}

	freshDecisions := createdDecisions

	if len(freshDecisions) > 0 {
		for _, decision := range freshDecisions {
			planner.retainCandidate(decision)
		}

		retireDecisionGraphs(thesis, freshDecisions)
	}

	// Structural candidates survive a temporarily thin book. They are re-priced
	// on every planner pass, including passes where no new graph completed, so a
	// later executable quote can admit the already-supported thesis. Fresh
	// rejections remain in the round as observable decisions instead of vanishing
	// merely because they are not retained for execution.
	createdDecisions = planner.candidateCopies()

	for _, decision := range freshDecisions {
		if decision != nil && decision.Action == types.ActionNothing {
			createdDecisions = append(createdDecisions, decision)
		}
	}

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

	if planner.candidates == nil {
		planner.candidates = make(map[string]*types.Decision)
	}

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
	candidate.EntryCost = nil
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

		if !retained.PredictiveReady {
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

func decisionTrace(
	graph *logicgraph.Graph,
	root *mcts.Node,
	recommended float64,
	iterations int,
) *types.DecisionTrace {
	summary := graph.OpportunitySummary()
	branches := make([]types.DecisionMCTSBranch, 0, len(root.Children))
	roots := graph.Roots()

	for _, branch := range root.Children {
		meanReward := 0.0

		if branch.Visits > 0 {
			meanReward = branch.TotalReward / float64(branch.Visits)
		}

		branches = append(branches, types.DecisionMCTSBranch{
			Action:     graphActionLabel(roots, branch.Action),
			Visits:     branch.Visits,
			MeanReward: meanReward,
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
		Hypothesis:       summary.Hypothesis,
		GraphSupports:    summary.Support,
		GraphContradicts: summary.Contradiction,
		GraphConditions:  summary.Conditions,
		ThesisBalance:    summary.Balance,
		ThesisConfidence: summary.Confidence,
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

/*
predictiveReadiness states whether predictive coding has earned the right to
participate in admission. MCTS still runs while this is false so the UI can
show the structural alternatives, but no capital is committed until the task
head is at least baseline-skilled and owns a supported transition horizon.
*/
func predictiveReadiness(graph *logicgraph.Graph) (bool, string) {
	if graph == nil {
		return false, "market graph unavailable"
	}

	if !graph.TaskSkillReady {
		return false, "task skill is still calibrating"
	}

	if graph.TaskSkill < 0.5 {
		return false, "task skill is below the zero-return baseline"
	}

	if graph.Forecast == nil || !graph.Forecast.Ready {
		return false, "no calibrated regime-transition posterior is published"
	}

	if graph.ForecastHorizon < 1 {
		return false, "no transition horizon is statistically supported"
	}

	return true, "baseline-or-better task skill with a supported transition horizon"
}

/*
reserveQualification protects the two reserve slots for abrupt, one-horizon
opportunities. A broad structural opportunity remains visible through
OpportunityType, but it cannot consume emergency capacity unless predictive
coding independently supports the shortest transition and the category is an
actual sudden-pump precursor.
*/
func reserveQualification(
	opportunityType string,
	predictiveReady bool,
	horizon int,
) (bool, string) {
	if !predictiveReady {
		return false, "predictive coder is not ready"
	}

	if horizon != 1 {
		return false, "reserve lane requires the shortest supported horizon"
	}

	switch types.CategoryType(opportunityType) {
	case types.VerticalIgnition, types.RiskOnSurge:
		return true, "sudden-pump precursor with one-horizon predictive support"
	default:
		return false, "structural opportunity is not an emergency reserve setup"
	}
}

/*
graphOpportunityType names a precursor family only when one of the graph's
supporting category nodes already carries that semantics. It does not infer an
opportunity type from price movement or from a generic positive score.
*/
func graphOpportunityType(graph *logicgraph.Graph) string {
	if graph == nil || graph.DecisionTarget == "" {
		return ""
	}

	preferred := map[types.CategoryType]bool{
		types.VerticalIgnition:  true,
		types.RiskOnSurge:       true,
		types.CoiledCompression: true,
		types.InefficientLag:    true,
		types.HiddenAbsorption:  true,
		types.AggressiveDrive:   true,
		types.OrganicTrend:      true,
		types.DecoupledAlpha:    true,
		types.EndogenousAlpha:   true,
		types.LiquidityVacuum:   true,
		types.ExtremeScarcity:   true,
	}

	best := types.CategoryTypeNone
	bestMass := 0.0

	for _, edge := range graph.Edges {
		if edge == nil || edge.To != graph.DecisionTarget ||
			edge.Relation != logicgraph.RelationSupports {
			continue
		}

		node := graph.Nodes[edge.From]

		if node == nil || node.Kind != logicgraph.KindCategory {
			continue
		}

		categoryValue, _ := node.Metadata["type"].(string)
		category := types.CategoryType(categoryValue)

		if !preferred[category] {
			continue
		}

		mass := edge.Weight * edge.Confidence

		if mass > bestMass || best == types.CategoryTypeNone {
			best = category
			bestMass = mass
		}
	}

	return string(best)
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

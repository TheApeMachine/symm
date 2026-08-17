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
	"golang.org/x/sync/errgroup"
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
	theses     chan *types.Thesis

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
		ctx:        ctx,
		cancel:     cancel,
		status:     types.READY,
		ui:         uiHub,
		recorder:   recorder,
		mctsEngine: newMCTSEngine(system.Cfg.Snapshot()),
		allocation: NewAllocation(ctx, desk),
		desk:       desk,
		theses:     make(chan *types.Thesis, 1),
		candidates: make(map[string]*types.Decision),
	}

	go planner.loop()

	return planner
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) HasCapacity() bool {
	if planner == nil || planner.desk == nil {
		return true
	}

	return planner.desk.OpenSlots(false) > 0
}

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

func (planner *Planner) Enqueue(thesis *types.Thesis) {
	if planner == nil || thesis == nil {
		return
	}

	select {
	case planner.theses <- thesis:
	default:
		select {
		case <-planner.theses:
		default:
		}

		select {
		case planner.theses <- thesis:
		default:
		}
	}
}

func (planner *Planner) loop() {
	for {
		select {
		case <-planner.ctx.Done():
			return
		case thesis, ok := <-planner.theses:
			if !ok {
				return
			}

			if err := planner.Update(thesis); err != nil {
				errnie.Error(errnie.Err(
					errnie.Internal,
					"planner: background update failed",
					err,
				))
			}
		}
	}
}

func (planner *Planner) readySymbols(thesis *types.Thesis) []*types.Symbol {
	if thesis == nil || thesis.Symbols == nil {
		return nil
	}

	ready := make([]*types.Symbol, 0)

	thesis.Symbols.Range(func(key, value any) bool {
		symbolState, ok := value.(*types.Symbol)

		if !ok || symbolState == nil {
			return true
		}

		symbolName, ok := key.(string)

		if !ok || symbolName == "" || isExcludedSymbol(symbolName) {
			return true
		}

		stored, found := symbolState.Graphs.Load("market_graph")

		if !found {
			return true
		}

		graph, valid := stored.(*logicgraph.Graph)

		if !valid || graph == nil || !graph.ReadyForSearch() {
			return true
		}

		ready = append(ready, symbolState)
		return true
	})

	return ready
}

func (planner *Planner) evaluateSymbol(
	symbolState *types.Symbol,
	config *system.Config,
) (*types.Decision, error) {
	symbol := symbolState.Symbol
	stored, found := symbolState.Graphs.Load("market_graph")

	if !found {
		return nil, nil
	}

	graph, valid := stored.(*logicgraph.Graph)

	if !valid || graph == nil {
		return nil, nil
	}

	cloned := graph.Clone()

	if cloned == nil || !cloned.ReadyForSearch() {
		return nil, nil
	}

	state, stateErr := mcts.NewGraphState(cloned)

	if stateErr != nil {
		return nil, fmt.Errorf("planner: graph state for %s: %w", symbol, stateErr)
	}

	history := state.History()
	mctsEngine := newMCTSEngine(config)

	searchStarted := time.Now()
	root, action, searchErr := mctsEngine.Search(
		state, config.Planner.MCTSIterations, history,
	)

	if planner.ObserveModule != nil {
		planner.ObserveModule("mcts", time.Since(searchStarted))
	}

	if searchErr != nil {
		return nil, fmt.Errorf("planner: graph search for %s: %w", symbol, searchErr)
	}

	decision := types.NewDecision(types.ActionNothing, symbol)
	decision.At = cloned.At
	decision.Forecast = cloned.Forecast
	decision.ForecastHorizon = cloned.ForecastHorizon
	decision.ForwardCurve = slices.Clone(cloned.ForwardCurve)

	perspective, perspectiveErr := graphPerspective(cloned)

	if perspectiveErr != nil {
		return nil, fmt.Errorf("planner: decision perspective for %s: %w", symbol, perspectiveErr)
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
	decision.OpportunityType = graphOpportunityType(cloned)
	decision.TaskSkill = cloned.TaskSkill
	decision.TaskSkillReady = cloned.TaskSkillReady
	decision.PredictiveReady, decision.PredictiveStatus = predictiveReadiness(cloned)
	decision.ReserveEligible, decision.ReserveReason = reserveQualification(
		decision.OpportunityType,
		decision.PredictiveReady,
		decision.ForecastHorizon,
	)
	decision.Opportunity = decision.ReserveEligible
	decision.Alternatives = make(map[string]float64)
	decision.Trace = decisionTrace(
		cloned,
		root,
		action,
		config.Planner.MCTSIterations,
	)

	for _, branch := range root.Children {
		if branch.Visits <= 0 {
			continue
		}

		reward := branch.TotalReward / float64(branch.Visits)
		decision.Alternatives[graphActionLabel(cloned.Roots(), branch.Action)] = reward

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
	case decision.OpportunityType == "":
		decision.Reason = "planner: no qualified structural opportunity precursor identified"
	default:
		decision.Action = types.ActionEnter
		decision.Cause = decision.OpportunityType
	}

	return decision, nil
}

func (planner *Planner) Update(thesis *types.Thesis) error {
	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return fmt.Errorf("planner: planner configuration required")
	}

	plannerStarted := time.Now()
	lastSearchEnd := plannerStarted

	defer func() {
		if planner.ObserveModule != nil {
			planner.ObserveModule("planner", time.Since(plannerStarted))
		}
	}()

	readySymbols := planner.readySymbols(thesis)

	if len(readySymbols) == 0 && !planner.hasCandidates() {
		return nil
	}

	createdDecisions := make([]*types.Decision, 0, len(readySymbols))

	if len(readySymbols) > 0 {
		var decisionMu sync.Mutex
		parentCtx := planner.ctx

		if parentCtx == nil {
			parentCtx = context.Background()
		}

		group, ctx := errgroup.WithContext(parentCtx)

		for _, symbolState := range readySymbols {
			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				decision, searchErr := planner.evaluateSymbol(symbolState, config)

				if searchErr != nil {
					return searchErr
				}

				if decision != nil {
					decisionMu.Lock()
					createdDecisions = append(createdDecisions, decision)
					decisionMu.Unlock()
				}

				return nil
			})
		}

		if err := group.Wait(); err != nil {
			return err
		}
	}

	freshDecisions := createdDecisions

	if len(freshDecisions) > 0 {
		for _, decision := range freshDecisions {
			planner.retainCandidate(decision)
		}

		retireDecisionGraphs(thesis, freshDecisions)
	}

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

		if err := planner.allocation.Calculate(createdDecisions); err != nil {
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

	if err := audit.Record(planner.recorder, decisions); err != nil {
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

	if err := planner.executeDecisions(createdDecisions, lastSearchEnd); err != nil {
		return err
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

func (planner *Planner) executeDecisions(
	createdDecisions []*types.Decision,
	lastSearchEnd time.Time,
) error {
	winners := make([]*types.Decision, 0, len(createdDecisions))

	for _, decision := range createdDecisions {
		if decision.Action != types.ActionEnter {
			planner.removeCandidate(decision.Symbol)
			continue
		}

		winners = append(winners, decision)
	}

	if planner.desk == nil {
		return nil
	}

	slices.SortFunc(winners, admissionOrder)

	for _, decision := range winners {
		executeStarted := time.Now()

		if planner.ObserveHop != nil {
			planner.ObserveHop("allocation", "desk", executeStarted.Sub(lastSearchEnd))
		}

		var err error

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

	return nil
}

func newMCTSEngine(config *system.Config) *mcts.CausalMCTS {
	engine := mcts.NewCausalMCTS(
		mcts.DefaultCausalEngine{},
		math.Sqrt2,
		1,
		len(mcts.GraphFeatureColumns)+1,
		mcts.GraphTreatmentColumn,
		mcts.GraphTargetColumn,
		mcts.GraphControlColumns,
		mcts.GraphFeatureColumns,
		false,
	)

	if config != nil && config.Planner != nil {
		engine.C = config.Planner.ExplorationConstant
		engine.CausalAlpha = config.Planner.CausalAlpha
	}

	return engine
}

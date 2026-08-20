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
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
	logicgraph "github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Planner struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	ui         *transport.MapReduce[[]byte]
	recorder   *audit.Recorder
	mctsEngine *mcts.CausalMCTS
	allocation *Allocation
	desk       *broker.Desk
	thesis     *types.Thesis

	candidateMu sync.Mutex
	candidates  map[string]*types.Decision
	lastTick    int64

	ObserveModule func(string, time.Duration)
	ObserveHop    func(string, string, time.Duration)
	executeEntry  func(types.Decision) error
}

func NewPlanner(
	ctx context.Context,
	uiHub *transport.MapReduce[[]byte],
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
		thesis:     thesis,
		candidates: make(map[string]*types.Decision),
	}

	go planner.run()
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

/*
plannerRest parks the planner loop between measurement passes. It is not a
time horizon: the loop wakes immediately when a new thesis tick is observed;
the sleep only prevents a bare spin when the market is producing no new rows.
*/
const plannerRest = 5 * time.Millisecond

func (planner *Planner) run() {
	for {
		select {
		case <-planner.ctx.Done():
			return
		default:
		}

		var tick int64

		if planner.thesis != nil {
			tick = planner.thesis.Tick
		}

		if tick == planner.lastTick && !planner.hasCandidates() {
			time.Sleep(plannerRest)
			continue
		}

		planner.lastTick = tick

		if err := planner.Update(planner.thesis); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"planner: background update failed",
				err,
			))
		}
	}
}

/*
readySymbols is the planner's pre-scan over the whole thesis: it cheaply walks
every symbol's market graph and keeps only the ones whose decision proposition
has accumulated enough confidence to be worth a causal search. Sparse graphs
are left to accumulate more observations instead of spending search effort.
*/
func (planner *Planner) readySymbols(
	thesis *types.Thesis,
) map[string]*logicgraph.Graph {
	ready := make(map[string]*logicgraph.Graph)

	minimumConfidence := 0.0

	if config := system.Cfg.Snapshot(); config != nil && config.Planner != nil {
		minimumConfidence = config.Planner.MinimumConfidence
	}

	thesis.Symbols.Range(func(key, value any) bool {
		symbolState, ok := value.(*types.Symbol)

		if !ok || symbolState == nil {
			return true
		}

		symbolName, ok := key.(string)

		if !ok || symbolName == "" || isExcludedSymbol(symbolName) {
			return true
		}

		var graph *logicgraph.Graph

		for stored := range symbolState.MarketGraphs(types.SourceGraph) {
			graph = stored
		}

		if graph == nil || !graph.SearchableEnough(minimumConfidence) {
			return true
		}

		ready[symbolName] = graph
		return true
	})

	return ready
}

func (planner *Planner) decisionFromGraph(
	symbol string,
	cloned *logicgraph.Graph,
	config *system.Config,
	searchRoot *mcts.Node,
	recommended float64,
	iterations int,
) (*types.Decision, error) {
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
	decision.Confidence = perspective.TradeConfidence
	decision.PerspectiveConfidence = perspective.TradeConfidence
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

	if searchRoot != nil {
		decision.Trace = decisionTrace(
			cloned,
			searchRoot,
			recommended,
			iterations,
		)

		for _, branch := range searchRoot.Children {
			if branch.Visits <= 0 {
				continue
			}

			reward := branch.TotalReward / float64(branch.Visits)
			decision.Alternatives[graphActionLabel(cloned.Roots(), branch.Action)] = reward

			if branch.Action == recommended {
				decision.GraphScore = reward
			}
		}
	}

	switch {
	case decision.Direction <= 0 || decision.ThesisScore <= 0:
		decision.Reason = "planner: contradiction outweighs support for the long-opportunity thesis"
	case decision.Confidence < config.Planner.MinimumConfidence:
		decision.Reason = "planner: thesis does not clear the minimum confidence floor"
	case decision.ThesisScore < config.Planner.MinimumGraphScore:
		decision.Reason = "planner: structural thesis does not clear the regulated evidence boundary"
	default:
		if !decision.PredictiveReady {
			// Predictive coding enriches the observation space it is not a
			// veto over the structural graph evidence. The strategy reasons
			// over this alongside every other measurement.
			decision.PredictiveStatus = "enriching: " + decision.PredictiveStatus
		}

		// A named precursor type qualifies the reserve lane; its absence must
		// not veto an otherwise strong structural thesis. The category label
		// is a classification convenience, not additional evidence: direction,
		// thesis score, calibrated confidence, and predictive readiness have
		// already cleared, so the opportunity is admitted on that evidence.
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
	heldLegs := planner.heldLegs(thesis)

	if len(readySymbols) == 0 && len(heldLegs) == 0 && !planner.hasCandidates() {
		return nil
	}

	createdDecisions := make([]*types.Decision, 0, len(readySymbols))
	choices := make(map[string]float64, len(readySymbols))
	rejections := make([]struct {
		symbol string
		graph  *logicgraph.Graph
	}, 0)

	if len(readySymbols) > 0 || len(heldLegs) > 0 {
		legs := make([]portfolioLeg, 0, len(readySymbols)+len(heldLegs))
		graphs := make(map[string]*logicgraph.Graph, len(readySymbols))

		for symbolName, symbolGraph := range readySymbols {
			graph := symbolGraph

			if graph == nil || !graph.ReadyForSearch() {
				continue
			}

			summary := graph.OpportunitySummary()
			cloned := graph.Clone()

			if !summary.Ready || summary.Score <= 0 {
				rejections = append(rejections, struct {
					symbol string
					graph  *logicgraph.Graph
				}{symbolName, cloned})
				continue
			}

			graphs[symbolName] = cloned
			now := cloned.At
			opportunityType := graphOpportunityType(cloned)
			predictiveReady, _ := predictiveReadiness(cloned)
			reserveEligible, _ := reserveQualification(
				opportunityType,
				predictiveReady,
				cloned.ForecastHorizon,
			)

			legs = append(legs, portfolioLeg{
				Symbol:          symbolName,
				Summary:         summary,
				Opportunity:     cloned.ActiveOpportunity(now),
				Trust:           cloned.MeanTrust(now),
				ReserveEligible: reserveEligible,
			})
		}

		// Held lots join the same search so the tree can retire one that has
		// stopped earning its slot in favour of a stronger flat candidate.
		legs = append(legs, heldLegs...)

		// Without a desk the planner still evaluates the round; capacity must
		// not collapse the search, so the leg count itself is the bound.
		normalSlots := planner.normalSlots()

		if planner.desk == nil {
			normalSlots = len(legs)
		}

		reserveSlots := planner.reserveSlots()

		searchRoot, searchErr := portfolioSearch(
			NewPortfolioState(legs, normalSlots, reserveSlots),
			config.Planner.MCTSIterations*max(1, len(legs)),
		)

		if searchErr != nil {
			return searchErr
		}

		choices = planner.portfolioChoices(searchRoot, len(legs))

		for index, leg := range legs {
			if leg.Held {
				if action := choices[leg.Symbol]; action == portfolioExitReference(index) {
					decision := types.NewDecision(types.ActionExit, leg.Symbol)
					decision.At = thesis.At
					decision.Cause = "continuation value negative"
					decision.Reason = "planner: MCTS retired the held lot to free its slot"
					decision.GraphScore = -leg.Summary.Score
					decision.Trace = decisionTracePortfolio(searchRoot, leg.Symbol)
					createdDecisions = append(createdDecisions, decision)
				}

				continue
			}

			cloned := graphs[leg.Symbol]

			if cloned == nil {
				continue
			}

			decision, decisionErr := planner.decisionFromGraph(
				leg.Symbol,
				cloned,
				config,
				searchRoot,
				choices[leg.Symbol],
				config.Planner.MCTSIterations,
			)

			if decisionErr != nil {
				return decisionErr
			}

			if decision == nil {
				continue
			}

			if action := choices[leg.Symbol]; action == portfolioEnterReference(index) {
				decision.Action = types.ActionEnter
				decision.Cause = decision.OpportunityType
			} else {
				decision.Reason = "planner: MCTS did not select this candidate for the available slots"
			}

			createdDecisions = append(createdDecisions, decision)
		}

		// Contradicted or not-yet-ready graphs still owe the round an
		// observable rejection instead of vanishing from the decision set.
		for _, rejection := range rejections {
			if rejection.graph == nil {
				continue
			}

			decision, decisionErr := planner.decisionFromGraph(
				rejection.symbol,
				rejection.graph,
				config,
				nil,
				0,
				0,
			)

			if decisionErr != nil {
				return decisionErr
			}

			if decision != nil {
				createdDecisions = append(createdDecisions, decision)
			}
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
		if decision != nil && decision.Action != types.ActionEnter {
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
		symbol.Decisions.Push(*decision)
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
	exits := make([]*types.Decision, 0, len(createdDecisions))

	for _, decision := range createdDecisions {
		switch decision.Action {
		case types.ActionEnter:
			winners = append(winners, decision)
		case types.ActionExit:
			exits = append(exits, decision)
		default:
			planner.removeCandidate(decision.Symbol)
		}
	}

	if planner.desk == nil {
		return nil
	}

	for _, decision := range exits {
		if err := planner.desk.Execute(*decision); err != nil {
			decision.Reason = "planner: exit is no longer executable: " + err.Error()

			if !errnie.IsNotAcceptable(err) {
				return fmt.Errorf("planner: execute exit %s: %w", decision.Symbol, err)
			}
		}
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

/*
heldLegs builds one portfolio leg per open desk position so the same search that
admits new entries can decide which held lot has decayed past its slot.
A lot without a fresh graph still participates with a neutral summary: the only
question the search must answer for it is whether freeing the slot is worth it.
*/
func (planner *Planner) heldLegs(thesis *types.Thesis) []portfolioLeg {
	if planner == nil || planner.desk == nil || thesis == nil {
		return nil
	}

	held := make([]portfolioLeg, 0)

	for position := range planner.desk.Positions() {
		if position == nil || position.Decision.Symbol == "" {
			continue
		}

		status := position.Status.Load()

		if status != nil && *status == types.CLOSED {
			continue
		}

		symbol := thesis.Symbol(position.Decision.Symbol)

		if symbol == nil {
			continue
		}

		summary := logicgraph.OpportunitySummary{}

		for graph := range symbol.MarketGraphs(types.SourceGraph) {
			if graph != nil {
				summary = graph.OpportunitySummary()
			}
		}

		held = append(held, portfolioLeg{
			Symbol:  position.Decision.Symbol,
			Summary: summary,
			Held:    true,
		})
	}

	return held
}

/*
normalSlots and reserveSlots expose the desk capacity the portfolio state
consumes. A nil desk is treated as capacity-free so the planner's decision
logic still evaluates under test without a broker.
*/
func (planner *Planner) normalSlots() int {
	if planner == nil || planner.desk == nil {
		return 0
	}

	return planner.desk.OpenSlots(false)
}

func (planner *Planner) reserveSlots() int {
	if planner == nil || planner.desk == nil {
		return 0
	}

	return planner.desk.OpenSlots(true) - planner.desk.OpenSlots(false)
}

/*
portfolioChoices walks the principal variation and records the first genuine
intervention selected for each leg. Later hold branches on the same variation
are follow-on positions, not reversals of the entry choice, so the first enter
or exit is the decision that owns the round.
*/
func (planner *Planner) portfolioChoices(
	root *mcts.Node,
	legCount int,
) map[string]float64 {
	choices := make(map[string]float64, legCount)
	settled := make(map[string]bool, legCount)

	for index := 0; index < legCount; index++ {
		choices[portfolioSymbol(root, index)] = portfolioHoldReference(index)
	}

	current := root

	for current != nil {
		if index, intervened := decodePortfolioAction(current.Action); intervened {
			symbol := portfolioSymbol(root, index)

			if !settled[symbol] {
				choices[symbol] = current.Action
				settled[symbol] = true
			}
		}

		if len(current.Children) == 0 {
			break
		}

		current = mostVisitedPortfolioChild(current)
	}

	return choices
}

/*
portfolioSymbol resolves one index back to its leg symbol by walking the root
state's legs. The index is positional within this search round only.
*/
func portfolioSymbol(root *mcts.Node, index int) string {
	if root == nil || root.State == nil {
		return ""
	}

	state, supported := root.State.(*PortfolioState)

	if !supported || index < 0 || index >= len(state.legs) {
		return ""
	}

	return state.legs[index].Symbol
}

/*
decodePortfolioAction splits a composite action into its leg index and whether
it was a real enter/exit intervention (hold and done are structural, not held).
*/
func decodePortfolioAction(action float64) (int, bool) {
	if action == portfolioDoneAction {
		return 0, false
	}

	index := int(math.Floor((action - 1) / 3))
	kind := math.Mod(action, 3)

	if kind == 0 {
		kind = 3
	}

	return index, kind == portfolioEnterOffset || kind == portfolioExitOffset
}

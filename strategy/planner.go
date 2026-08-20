package strategy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	logicgraph "github.com/theapemachine/symm/types"
)

// strategyWireBranchCount matches the two ranked branch slots rendered for
// each live candidate decision.
const strategyWireBranchCount = 2

type Planner struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	status     types.Status
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
	thesis *types.Thesis,
	recorder *audit.Recorder,
	desk *broker.Desk,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	planner := &Planner{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.READY,
		recorder:   recorder,
		mctsEngine: newMCTSEngine(system.Cfg.Snapshot()),
		allocation: NewAllocation(ctx, desk),
		desk:       desk,
		thesis:     thesis,
		candidates: make(map[string]*types.Decision),
	}

	return planner
}

func (planner *Planner) Name() string { return "planner" }

func (planner *Planner) Error() error { return planner.err }

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
Run evaluates completed graph passes until cancellation or the first planner
error. Construction only wires the planner; the system supervisor owns this
lifecycle.
*/
func (planner *Planner) Run() error {
	for planner.err == nil {
		work := planner.thesis.Work(types.SourcePlanner)
		_, available := work.WaitPop(
			planner.ctx,
			string(types.SourcePlanner),
		)

		if !available {
			return planner.ctx.Err()
		}

		for range work.Drain(string(types.SourcePlanner), func(*types.Symbol) bool {
			return true
		}) {
		}

		tick := planner.thesis.Tick
		planner.lastTick = tick

		if err := planner.Update(planner.thesis); err != nil {
			planner.err = errnie.Error(errnie.Err(
				errnie.Internal,
				"planner: background update failed",
				err,
			))
		}
	}

	return planner.err
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

	defer func() {
		if planner.ObserveModule != nil {
			planner.ObserveModule("planner", time.Since(plannerStarted))
		}
	}()

	var err error
	evaluated := false

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, symbolOK := key.(string)
		symbolState, stateOK := value.(*types.Symbol)

		if !symbolOK || symbolName == "" || !stateOK || symbolState == nil ||
			isExcludedSymbol(symbolName) {
			return true
		}

		consumedGraph := false

		for graph := range symbolState.MarketGraphs(types.SourcePlanner) {
			consumedGraph = true

			if graph == nil || !graph.SearchableEnough(config.Planner.MinimumConfidence) {
				continue
			}

			evaluated = true
			err = planner.updateGraph(thesis, config, symbolName, graph)

			if err != nil {
				return false
			}
		}

		if consumedGraph && symbolState.HasGraphInputs() {
			thesis.Work(types.SourceGraph).Push(symbolState)
		}

		return err == nil
	})

	if err != nil || evaluated {
		return err
	}

	return planner.updateGraph(thesis, config, "", nil)
}

func (planner *Planner) updateGraph(
	thesis *types.Thesis,
	config *system.Config,
	symbolName string,
	graph *logicgraph.Graph,
) error {
	readySymbols := make(map[string]*logicgraph.Graph, 1)

	if graph != nil {
		readySymbols[symbolName] = graph
	}

	lastSearchEnd := time.Now()
	heldLegs := planner.heldLegs(readySymbols)

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
			if planner.Holding(symbolName) {
				continue
			}

			graph := symbolGraph

			if graph == nil || !graph.ReadyForSearch() {
				continue
			}

			summary := graph.OpportunitySummary()

			if !summary.Ready || summary.Score <= 0 {
				rejections = append(rejections, struct {
					symbol string
					graph  *logicgraph.Graph
				}{symbolName, graph})
				continue
			}

			graphs[symbolName] = graph
			now := graph.At
			opportunityType := graphOpportunityType(graph)
			predictiveReady, _ := predictiveReadiness(graph)
			reserveEligible, _ := reserveQualification(
				opportunityType,
				predictiveReady,
				graph.ForecastHorizon,
			)

			legs = append(legs, portfolioLeg{
				Symbol:          symbolName,
				Summary:         summary,
				Opportunity:     graph.ActiveOpportunity(now),
				Trust:           graph.MeanTrust(now),
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
		planner.publishStrategy(thesis, false, "accumulating", decisions)

		return nil
	}

	if err := planner.executeDecisions(createdDecisions, lastSearchEnd); err != nil {
		return err
	}

	decisions = decisions[:0]

	for _, decision := range createdDecisions {
		decisions = append(decisions, *decision)
	}

	planner.publishStrategy(thesis, true, "decisions", decisions)

	return nil
}

func (planner *Planner) publishStrategy(
	thesis *types.Thesis,
	evaluated bool,
	outcome string,
	decisions []types.Decision,
) {
	rows := make([]*wire.DecisionT, 0, len(decisions))

	for _, decision := range decisions {
		rows = append(rows, types.DecisionWire(
			decision,
			strategyWireBranchCount,
			false,
		))
	}

	thesis.Publish(&wire.FrameT{
		Type: wire.FrameStrategyFrame,
		Value: &wire.StrategyFrameT{
			Evaluated: evaluated,
			Outcome:   outcome,
			Decisions: rows,
		},
	})
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
A held lot uses the graph already consumed by the current evaluation pass, so
the planner never reads the stream twice or retains a shadow graph state.
*/
func (planner *Planner) heldLegs(
	graphs map[string]*logicgraph.Graph,
) []portfolioLeg {
	if planner == nil || planner.desk == nil {
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

		if position.Holding != nil && position.Holding.Stoploss != nil &&
			position.Holding.Stoploss.Status == types.TRIGGERED {
			continue
		}

		graph := graphs[position.Decision.Symbol]

		if graph == nil {
			continue
		}

		summary := logicgraph.OpportunitySummary{}
		summary = graph.OpportunitySummary()

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

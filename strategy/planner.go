package strategy

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/transport"
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
	stager     *audit.Stager
	mctsEngine *mcts.CausalMCTS
	allocation *Allocation
	desk       *broker.Desk
	thesis     *types.Thesis
	work       *transport.Consumer[*types.Symbol]

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
		stager:     audit.NewStager(recorder),
		mctsEngine: newMCTSEngine(system.Cfg.Snapshot()),
		allocation: NewAllocation(ctx, desk),
		desk:       desk,
		thesis:     thesis,
	}
	planner.work = transport.NewConsumer[*types.Symbol](planner.Name(), planner.consume)
	thesis.Work(types.SourcePlanner).Register(planner.work)

	return planner
}

func (planner *Planner) Name() string { return "planner" }

func (planner *Planner) Error() error { return planner.err }

func (planner *Planner) Stager() *audit.Stager {
	return planner.stager
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

func (planner *Planner) consume() {
	go func() {
		work := planner.thesis.Work(types.SourcePlanner)
		remaining := work.Length(planner.work)

		for range work.Drain(planner.work, func(_ *types.Symbol) bool {
			return remaining > 0
		}) {
			select {
			case <-planner.ctx.Done():
				planner.err = planner.ctx.Err()
				return
			default:
			}

			remaining--

			if remaining > 0 {
				continue
			}

			if err := planner.Update(planner.thesis); err != nil {
				planner.err = errnie.Error(errnie.Err(
					errnie.Internal,
					"planner: background update failed",
					err,
				))
				planner.thesis.Fail(planner.err)
				return
			}
		}
	}()
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
	opportunity := cloned.ActiveOpportunity(cloned.At)
	decision.OpportunityType = string(opportunity.Type)
	decision.TaskSkill = cloned.TaskSkill
	decision.TaskSkillReady = cloned.TaskSkillReady
	decision.PredictiveReady, decision.PredictiveStatus = predictiveReadiness(cloned)

	decision.ReserveEligible, decision.ReserveReason = reserveQualification(
		opportunity.Type,
		decision.PredictiveReady,
		decision.ForecastHorizon,
	)
	decision.Opportunity = opportunity.Type != types.OpportunityNone
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
			decision.Alternatives[portfolioActionLabel(searchRoot, cloned.Roots(), branch.Action)] = reward

			if branch.Action == recommended {
				decision.GraphScore = reward
			}
		}
	}

	applyAdmission(decision, config.Planner.Admission, cloned)

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

	readySymbols := make(map[string]*logicgraph.Graph)

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, symbolOK := key.(string)
		symbolState, stateOK := value.(*types.Symbol)

		if !symbolOK || symbolName == "" || !stateOK || symbolState == nil ||
			isExcludedSymbol(symbolName) {
			return true
		}

		consumedGraph := false

		for graph := range symbolState.MarketGraphs(
			symbolState.GraphConsumers[types.GraphConsumerPlanner],
		) {
			consumedGraph = true

			if graph == nil || !graph.ReadyForSearch() {
				continue
			}

			readySymbols[symbolName] = graph
		}

		if consumedGraph && symbolState.HasGraphInputs() {
			thesis.ScheduleWork(types.SourceGraph, symbolState)
		}

		return true
	})

	if len(readySymbols) == 0 {
		return nil
	}

	return planner.updateGraph(thesis, config, readySymbols)
}

func (planner *Planner) updateGraph(
	thesis *types.Thesis,
	config *system.Config,
	readySymbols map[string]*logicgraph.Graph,
) error {
	lastSearchEnd := time.Now()

	if len(readySymbols) == 0 {
		return nil
	}

	createdDecisions := make([]*types.Decision, 0, len(readySymbols))
	legs := make([]portfolioLeg, 0, len(readySymbols))
	graphs := make(map[string]*logicgraph.Graph, len(readySymbols))
	admitted := make(map[string]*types.Decision, len(readySymbols))

	for symbolName, symbolGraph := range readySymbols {
		if planner.Holding(symbolName) {
			continue
		}

		graph := symbolGraph

		if graph == nil || !graph.ReadyForSearch() {
			continue
		}

		seed, err := planner.decisionFromGraph(
			symbolName,
			graph,
			config,
			nil,
			0,
			0,
		)

		if err != nil {
			return err
		}

		if seed == nil {
			continue
		}

		if seed.Reason != "" {
			createdDecisions = append(createdDecisions, seed)
			continue
		}

		seed.Action = types.ActionEnter
		admitted[symbolName] = seed
		graphs[symbolName] = graph
		opportunity := graph.ActiveOpportunity(graph.At)
		reserveEligible, _ := reserveQualification(
			opportunity.Type,
			seed.PredictiveReady,
			seed.ForecastHorizon,
		)
		liquidity := alternativesOf(seed)[liquidityScoreKey]
		liquidityMass := alternativesOf(seed)[liquidityMassKey]
		legs = append(legs, portfolioLeg{
			Symbol:          symbolName,
			Summary:         graph.OpportunitySummary(),
			Opportunity:     opportunity,
			ReserveEligible: reserveEligible,
			Liquidity:       liquidity,
			LiquidityMass:   liquidityMass,
		})
	}

	if len(legs) > 0 {
		seeds := make([]*types.Decision, 0, len(admitted))

		for _, decision := range admitted {
			seeds = append(seeds, decision)
		}

		rankAdmissionCandidates(seeds)
		slices.SortFunc(legs, func(left, right portfolioLeg) int {
			return admissionOrder(admitted[left.Symbol], admitted[right.Symbol])
		})

		normalSlots := planner.normalSlots()

		if planner.desk == nil {
			normalSlots = len(legs)
		}

		reserveSlots := planner.reserveSlots()
		searchStarted := time.Now()
		searchRoot, searchErr := portfolioSearch(
			NewPortfolioState(legs, normalSlots, reserveSlots),
			config.Planner.MCTSIterations*max(1, len(legs)),
		)

		if planner.ObserveModule != nil {
			planner.ObserveModule("mcts", time.Since(searchStarted))
		}

		lastSearchEnd = time.Now()

		if searchErr != nil {
			return searchErr
		}

		for index, leg := range legs {
			cloned := graphs[leg.Symbol]

			if cloned == nil {
				continue
			}

			decision, decisionErr := planner.decisionFromGraph(
				leg.Symbol,
				cloned,
				config,
				searchRoot,
				portfolioEnterReference(index),
				config.Planner.MCTSIterations,
			)

			if decisionErr != nil {
				return decisionErr
			}

			if decision == nil {
				continue
			}

			if decision.Reason == "" {
				if !decision.Opportunity && !decision.PredictiveReady && !decision.ReserveEligible {
					decision.Action = types.ActionNothing
					decision.GraphScore = 0
					decision.Trace = nil
					decision.Reason = "planner: no actable opportunity"

					if !decision.PredictiveReady && decision.PredictiveStatus != "" {
						decision.Reason += "; predictive state is informational: " +
							decision.PredictiveStatus
					}
				} else {
					decision.Action = types.ActionEnter
					decision.Cause = decision.OpportunityType
					decision.Reason = ""
				}
			}

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
		symbol.Decisions.PushLatest(*decision)
	}

	decisions := make([]types.Decision, 0, len(createdDecisions))
	actionable := false

	for _, decision := range createdDecisions {
		decisions = append(decisions, *decision)

		if decision.Action != types.ActionNothing {
			actionable = true
		}
	}

	for index := range decisions {
		// Stage with a default 10 minute horizon for hindsight evaluation.
		planner.stager.Stage(&decisions[index], 10*time.Minute)
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

package strategy

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

type Planner struct {
	*types.Actor
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	api       *websocket.API
	desk      *broker.Desk
	price     *broker.Price
	balance   *broker.Balance
	analyzer  *logic.Analyzer
	recorder  *audit.Recorder
	evaluator Evaluator
	arbiter   *Arbiter
	allocator *Allocator
}

func NewPlanner(
	ctx context.Context,
	uiHub chan<- []byte,
	api *websocket.API,
	desk *broker.Desk,
	instrument *broker.Instrument,
	price *broker.Price,
	balance *broker.Balance,
	analyzer *logic.Analyzer,
	allocator *Allocator,
	recorder *audit.Recorder,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)
	_ = uiHub // Retained for signature compatibility

	evaluator := NewEvaluator()
	arbiter := NewArbiter(desk, price)

	if allocator == nil {
		allocator = NewAllocator(ctx, balance, instrument, price)
	}

	planner := &Planner{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		api:       api,
		desk:      desk,
		price:     price,
		balance:   balance,
		analyzer:  analyzer,
		recorder:  recorder,
		evaluator: evaluator,
		arbiter:   arbiter,
		allocator: allocator,
	}

	planner.Actor = types.NewActor(ctx, "planner", map[string]types.Handler{
		"thesis": {Topic: "thesis", Fn: planner.onThesis},
	})

	return planner
}

func (p *Planner) Initialize(analyzer *logic.Analyzer) error {
	errnie.Info("initializing planner")

	p.Actor.InitializeSize(
		1,
		types.Topic{Name: "thesis", Actor: analyzer.Actor},
	)

	p.status = types.READY
	return nil
}

func (p *Planner) Status() types.Status {
	return p.status
}

func (p *Planner) Close() error {
	p.cancel()
	return nil
}

func (p *Planner) onThesis(msg any) any {
	thesis, ok := msg.(*types.Thesis)
	if !ok || thesis == nil {
		return nil
	}

	return p.Decide(thesis)
}

// Decide is the clean, 4-step pipeline executor.
func (p *Planner) Decide(thesis *types.Thesis) *types.Thesis {
	if thesis == nil {
		return thesis
	}

	errnie.Error(audit.Phase(p.recorder, thesis.Tick, "decide_begin", nil))

	// Step 1: Sync thesis lifecycle with live desk holdings
	syncThesisLifecycle(thesis, p.desk.Holdings())
	p.retainUnapplied(thesis)

	// Step 2: Score continuation for active holdings
	p.evaluator.ManageContinuation(thesis, p.desk, p.price)

	// Step 3: Check completeness
	if thesis.Incomplete() {
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: types.ActionNothing,
			Cause:  "measure_incomplete",
			Reason: "accumulated evidence is incomplete; refusing entries",
		})
		assignDecisionIDs(thesis)
		p.auditDecisions(thesis)
		errnie.Error(audit.Phase(p.recorder, thesis.Tick, "decide_end", decisionCounts(thesis.Decisions)))
		return thesis
	}

	// Step 4: Score new entry opportunities
	p.evaluator.EvaluateOpportunities(thesis, p.desk, p.price, p.balance)

	// Step 5: Portfolio Arbitration (Ranking, Slot Limits & Rotation/Displacement)
	p.arbiter.Arbitrate(thesis)

	// Step 6: Capital Allocation & Quantization
	if err := p.allocator.Allocate(thesis); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "failed to allocate", err))
	}

	assignDecisionIDs(thesis)
	p.auditDecisions(thesis)

	errnie.Error(audit.Phase(
		p.recorder,
		thesis.Tick,
		"decide_end",
		decisionCounts(thesis.Decisions),
	))

	return thesis
}

func (p *Planner) retainUnapplied(thesis *types.Thesis) {
	if thesis == nil {
		return
	}
	retained := make([]types.Decision, 0, len(thesis.Decisions))
	for _, decision := range thesis.Decisions {
		if decision.Action != types.ActionEnter {
			continue
		}
		if p.desk != nil {
			if holding, err := p.desk.Holding(decision.Symbol); err == nil && holding.Status != types.CLOSED {
				continue
			}
		}
		retained = append(retained, decision)
	}
	thesis.Decisions = retained
}

func (p *Planner) auditDecisions(thesis *types.Thesis) {
	if p == nil || thesis == nil {
		return
	}
	for _, decision := range thesis.Decisions {
		if decision.Symbol == "" {
			continue
		}
		lifecycle, _ := lifecycleState(thesis, decision.Symbol)
		_ = audit.StrategyDecision(p.recorder, thesis.Tick, lifecycle, decision)
	}
}

func assignDecisionIDs(thesis *types.Thesis) {
	if thesis == nil {
		return
	}
	for i := range thesis.Decisions {
		thesis.Decisions[i].EnsureID()
	}
}

func decisionCounts(decisions []types.Decision) map[string]any {
	counts := map[string]int{}
	for _, decision := range decisions {
		counts[string(decision.Action)]++
	}
	summary := map[string]any{"decisions": len(decisions)}
	for action, count := range counts {
		summary[action] = count
	}
	return summary
}

func syncThesisLifecycle(thesis *types.Thesis, live map[string]*types.Holding) {
	if thesis == nil {
		return
	}
	for symbol, holding := range live {
		current, _ := lifecycleState(thesis, symbol)
		next := holdingLifecycle(current, holding)
		if next != "" && next != current {
			thesis.NoteLifecycle(symbol, next, thesis.At)
		}
	}
}

func lifecycleState(thesis *types.Thesis, symbol string) (string, bool) {
	if thesis == nil || symbol == "" {
		return "", false
	}
	val, found := thesis.Lifecycle.Load(symbol)
	if !found {
		return "", false
	}
	state, ok := val.(string)
	return state, ok
}

func holdingLifecycle(current string, holding *types.Holding) string {
	if holding == nil {
		return current
	}
	switch holding.Status {
	case types.OPEN, types.FILLED, types.READY:
		return types.LifecycleManaging
	case types.CLOSED:
		return types.LifecycleClosed
	default:
		return current
	}
}

// PostMortem evaluates a completed thesis snapshot.
type PostMortem struct{}

func (pm *PostMortem) Evaluate(thesis *types.Thesis, symbol string) error {
	if val, ok := thesis.Lifecycle.Load(symbol); ok {
		if state, isStr := val.(string); isStr && state == types.LifecyclePostMortemReady {
			thesis.Lifecycle.Store(symbol, types.LifecycleEvaluated)
		}
	}
	return nil
}

package strategy

import (
	"context"
	"maps"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Planner records feasible actions from accumulated measurements.
*/
type Planner struct {
	*types.Actor
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	api         *websocket.API
	desk        *broker.Desk
	instrument  *broker.Instrument
	price       *broker.Price
	balance     *broker.Balance
	uiHub       chan<- []byte
	analyzer    *logic.Analyzer
	recorder    *audit.Recorder
	allocator   *Allocator
	opportunity *Opportunity
	admit       *Admit
	continuity  Continuity
	evidence    Evidence
	rotate      Rotate
	arbiter     *Arbiter
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
	rotate := NewRotate()
	admit := NewAdmit(ctx, balance, desk, rotate)

	planner := &Planner{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.READY,
		api:        api,
		desk:       desk,
		instrument: instrument,
		price:      price,
		balance:    balance,
		allocator:  allocator,
		uiHub:      uiHub,
		analyzer:   analyzer,
		recorder:   recorder,
		opportunity: NewOpportunity(
			ctx, cancel, price, balance, recorder, uiHub,
		),
		admit:      admit,
		continuity: NewContinuity(price, balance, rotate),
		evidence:   NewEvidence(),
		rotate:     rotate,
		arbiter:    NewArbiter(desk, price, balance, admit, rotate),
	}

	planner.Actor = types.NewActor(ctx, "planner", map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: planner.thesis},
		"trade":  {Topic: "trade", Fn: planner.thesis},
	})

	return planner
}

func (planner *Planner) Initialize(analyzer *logic.Analyzer) error {
	errnie.Info("initializing planner")

	planner.Actor.InitializeSize(
		1,
		types.Topic{Name: "ticker", Actor: analyzer.Actor},
		types.Topic{Name: "trade", Actor: analyzer.Actor},
	)

	planner.status = types.READY
	return nil
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) thesis(message any) any {
	return planner.Decide(message.(*types.Thesis))
}

/*
Update installs measurements onto the durable Thesis and runs the analyzer.
*/
func (planner *Planner) Update(
	thesis *types.Thesis,
	at time.Time,
	tick int64,
	rows []*types.Measurement,
) *types.Thesis {
	if thesis == nil {
		thesis = types.NewThesis()
	}

	thesis.ResetTick(at, tick)
	thesis.AppendMeasurements(rows)
	started := time.Now()

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "measure_end", map[string]any{
		"measurements": len(rows),
		"ns":           time.Since(started).Nanoseconds(),
	}))

	if planner.analyzer != nil {
		planner.analyzer.Update(thesis)
	}

	return thesis
}

func (planner *Planner) Decide(thesis *types.Thesis) *types.Thesis {
	if err := planner.validate(map[string]any{"thesis": thesis}); err != nil {
		return thesis
	}

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "decide_begin", nil))

	// Keep Enter rows Crypto has not yet opened on Balance. Clearing them while
	// LifecycleEntrySelected + thesis Holdings remained blocked Measure forever.
	planner.retainUnapplied(thesis)

	planner.opportunity.StampFriction(thesis)
	planner.continuity.Manage(thesis)

	if thesis.Incomplete() {
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: types.ActionNothing,
			Cause:  "measure_incomplete",
			Reason: "accumulated evidence is marked incomplete; refuse fresh enters",
		})

		errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "decide_end", map[string]any{
			"decisions": len(thesis.Decisions),
		}))

		return thesis
	}

	planner.opportunity.Measure(thesis)
	planner.arbiter.Select(thesis)

	if err := planner.allocator.Allocate(thesis); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "failed to allocate", err))
	}

	planner.rotate.Commit(thesis)

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "decide_end", map[string]any{
		"decisions": len(thesis.Decisions),
	}))

	return thesis
}

/*
retainUnapplied drops settled desk outcomes but keeps Enter decisions whose
symbol is not yet OPEN on Balance, so a slow Crypto.Apply is not erased by the
next Decide.
*/
func (planner *Planner) retainUnapplied(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	retained := make([]types.Decision, 0, len(thesis.Decisions))

	for _, decision := range thesis.Decisions {
		if decision.Action != types.ActionEnter {
			continue
		}

		if planner.balance != nil {
			if holding, err := planner.balance.Holding(decision.Symbol); err == nil &&
				holding.Status != types.CLOSED {
				continue
			}
		}

		retained = append(retained, decision)
	}

	thesis.Decisions = retained
}

func (planner *Planner) validate(mandatory map[string]any) error {
	check := map[string]any{
		"ctx":         planner.ctx,
		"cancel":      planner.cancel,
		"desk":        planner.desk,
		"balance":     planner.balance,
		"opportunity": planner.opportunity,
		"admit":       planner.admit,
		"arbiter":     planner.arbiter,
		"allocator":   planner.allocator,
		"uiHub":       planner.uiHub,
	}
	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}

package strategy

import (
	"context"
	"maps"
	"time"

	"github.com/theapemachine/datura"
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

	return &Planner{
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
}

func (planner *Planner) Initialize() error {
	errnie.Info("initializing planner")
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
		thesis = types.NewThesis(planner.uiHub)
	}

	thesis.ResetTick(at, tick)
	thesis.Measurements = rows
	started := time.Now()

	if len(thesis.Measurements) > 0 {
		select {
		case planner.uiHub <- datura.Map[any]{
			"measurements": types.ForPublish(thesis.Measurements),
		}.Marshal():
		default:
		}
	}

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "measure_end", map[string]any{
		"measurements": len(thesis.Measurements),
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

	planner.opportunity.StampFriction(thesis)
	planner.continuity.Manage(thesis)
	planner.evidence.Regulate(thesis, planner.balance)

	if thesis.Incomplete() {
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: types.ActionNothing,
			Cause:  "measure_incomplete",
			Reason: "accumulated evidence is marked incomplete; refuse fresh enters",
		})

		return thesis
	}

	planner.opportunity.Measure(thesis)
	planner.arbiter.Select(thesis)

	if err := planner.allocator.Allocate(thesis); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "failed to allocate", err))
	}

	planner.rotate.Commit(thesis)

	return thesis
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

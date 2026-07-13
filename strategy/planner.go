package strategy

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Planner currently selects entry intents by comparing buy utility with doing
nothing. Keeping this scope explicit prevents the entry path from masquerading
as the later position-aware hold, exit, rotation, and reversal strategy.
*/
type Planner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	status   types.Status
	uiHub    chan<- []byte
	signals  []types.Signal
	analyzer *logic.Analyzer
}

/*
NewPlanner creates a Planner that is ready once its dependencies are assigned.
Planning has no deferred initialization or warmup of its own.
*/
func NewPlanner(
	ctx context.Context,
	uiHub chan<- []byte,
	signals []types.Signal,
	analyzer *logic.Analyzer,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	return &Planner{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.READY,
		uiHub:    uiHub,
		signals:  signals,
		analyzer: analyzer,
	}
}

func (planner *Planner) Initialize() error {
	errnie.Info("initializing planner")

	planner.status = types.READY
	return nil
}

/*
Status reports whether the Planner itself is ready to evaluate evidence.
Boot-stage admission remains a separate concern enforced by Update.
*/
func (planner *Planner) Status() types.Status {
	return planner.status
}

/*
Update evaluates the thesis for all symbols and returns intended actions.
*/
func (planner *Planner) Update() *types.Thesis {
	thesis := types.NewThesis(planner.uiHub)

	for _, signal := range planner.signals {
		thesis = signal.Measure(thesis)
	}

	if planner.analyzer != nil {
		planner.analyzer.Update(thesis)
	}

	return thesis
}

package regulator

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Solver is the global predictive regulator. It pairs the exact controls applied
after one account valuation with the next changed valuation, learns that
temporal response, and publishes the next bounded intervention atomically.
*/
type Solver struct {
	mu              sync.Mutex
	configSource    *system.Config
	optimizer       *optimizer
	ui              chan []byte
	history         []float64
	historyCapacity int
	lastEquity      float64
	peakEquity      float64
	lastRevision    uint64
	hadExposure     bool
	lastResult      optimizationResult
}

/*
NewSolver constructs a validated predictive regulator over the current system
configuration.
*/
func NewSolver(
	_ context.Context,
	ui chan []byte,
) (*Solver, error) {
	configSource := system.Cfg

	if configSource == nil {
		return nil, fmt.Errorf("regulator: system configuration required")
	}

	config := configSource.Snapshot()

	if config == nil || config.Regulator == nil || config.Regulator.HistoryCapacity < 1 {
		return nil, fmt.Errorf("regulator: positive history capacity required")
	}

	model, err := newOptimizer(config)

	if err != nil {
		return nil, err
	}

	return &Solver{
		configSource:    configSource,
		optimizer:       model,
		ui:              ui,
		history:         make([]float64, 0, config.Regulator.HistoryCapacity),
		historyCapacity: config.Regulator.HistoryCapacity,
	}, nil
}

/*
Status reports synchronous construction readiness.
*/
func (solver *Solver) Status() types.Status {
	if solver == nil || solver.optimizer == nil {
		return types.ERROR
	}

	return types.READY
}

/*
Update consumes one new broker equity revision. A new unchanged valuation is a
real zero-return outcome; only repeated delivery of the same revision is ignored.
*/
func (solver *Solver) Update(thesis *types.Thesis, exposed bool) error {
	if solver == nil || solver.optimizer == nil || solver.configSource == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: initialized solver required",
			nil,
		))
	}

	if thesis == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: thesis required",
			nil,
		))
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	equity, revision, found := thesis.EquitySnapshot()

	if !found || equity.Equity == nil || equity.Equity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: complete positive account equity required",
			nil,
		))
	}

	if revision <= solver.lastRevision {
		return nil
	}

	currentEquity := equity.Equity.Float64()

	if !exposed && !solver.hadExposure {
		solver.lastEquity = currentEquity
		solver.peakEquity = max(solver.peakEquity, currentEquity)
		solver.lastRevision = revision
		return nil
	}

	periodReturn, drawdown := solver.financialFeedback(currentEquity)
	result, err := solver.optimizer.update(periodReturn, drawdown)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: predictive optimization failed",
			err,
		))
	}

	if err := solver.applyControls(result.controls); err != nil {
		return err
	}

	solver.lastEquity = currentEquity
	solver.lastRevision = revision
	solver.hadExposure = exposed
	solver.lastResult = result
	solver.recordHistory(result.surprise)
	payload := solver.buildPayload(periodReturn, result)

	if solver.ui != nil {
		utils.Publish(solver.ui, datura.NewMap("regulator", payload))
	}

	return nil
}

func (solver *Solver) applyControls(controls controlVector) error {
	config := solver.configSource.Snapshot()

	if err := solver.optimizer.space.apply(controls, config); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: selected control vector invalid",
			err,
		))
	}

	if err := solver.configSource.ApplyRegulation(
		*config.Manifold,
		*config.Planner,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: publish selected controls failed",
			err,
		))
	}

	return nil
}

func (solver *Solver) financialFeedback(currentEquity float64) (float64, float64) {
	if solver.lastEquity <= 0 {
		solver.peakEquity = currentEquity
		return 0, 0
	}

	periodReturn := math.Log(currentEquity / solver.lastEquity)

	if currentEquity > solver.peakEquity {
		solver.peakEquity = currentEquity
	}

	drawdown := math.Log(currentEquity / solver.peakEquity)
	return periodReturn, drawdown
}

func (solver *Solver) recordHistory(value float64) {
	if len(solver.history) == solver.historyCapacity {
		copy(solver.history, solver.history[1:])
		solver.history[len(solver.history)-1] = value
		return
	}

	solver.history = append(solver.history, value)
}

/*
Close satisfies the solver lifecycle contract; Solver owns no background work.
*/
func (solver *Solver) Close() error {
	return nil
}

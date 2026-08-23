package regulator

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Solver is the global predictive regulator. It pairs the exact controls applied
after one account valuation with the next changed valuation, learns that
temporal response, and publishes the next bounded intervention atomically.
*/
type observedPositionMark struct {
	positionID string
	value      float64
}

type Solver struct {
	mu                   sync.Mutex
	configSource         *system.Config
	optimizer            *optimizer
	ui                   *transport.MapReduce[*types.UIFrame]
	history              []float64
	historyCapacity      int
	lastEquity           float64
	peakEquity           float64
	lastRevision         uint64
	hadExposure          bool
	lastResult           optimizationResult
	resolved             int
	marks                map[string]observedPositionMark
	markSamples          uint64
	lastMarkSymbol       string
	lastMarkReturn       float64
	lastMarkDrawdown     float64
	lastMarkFloor        float64
	lastMarkSurge        bool
	lastMarkAt           time.Time
	lastMarkContext      markContext
	hindsightSamples     uint64
	lastHindsightContext hindsightContext
	markAcc              markAccumulator
	hindsightAcc         hindsightAccumulator
}

/*
NewSolver constructs a validated predictive regulator over the current system
configuration.
*/
func NewSolver(
	_ context.Context,
	ui *transport.MapReduce[*types.UIFrame],
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
		marks:           make(map[string]observedPositionMark),
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
Predicting reports whether the regulator has resolved enough control/equity
pairs to be producing posterior forecasts and selecting bounded interventions.
A nil or uninitialized solver is not predicting.
*/
func (solver *Solver) Predicting() bool {
	if solver == nil {
		return false
	}

	if solver.resolved < 3 {
		return false
	}

	return solver.lastResult.healthy()
}

/*
Update consumes one new broker equity revision. A new unchanged valuation is a
real zero-return outcome, and a flat interval is an explicit inactivity target;
only repeated delivery of the same revision is ignored.
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

	periodReturn, drawdown := solver.financialFeedback(currentEquity)
	marks := solver.markAcc.snapshot()
	hindsight := solver.hindsightAcc.snapshot()
	active := exposed || solver.hadExposure || marks.samples > 0
	result, err := solver.optimizer.update(periodReturn, drawdown, active, marks, hindsight)

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

	solver.markAcc.reset()
	solver.hindsightAcc.reset()
	solver.lastMarkContext = marks
	solver.lastHindsightContext = hindsight
	solver.lastEquity = currentEquity
	solver.lastRevision = revision
	solver.hadExposure = exposed
	solver.lastResult = result
	solver.resolved = solver.optimizer.resolvedCount()
	solver.recordHistory(result.surprise)
	payload := solver.buildPayload(periodReturn, result)

	if solver.ui != nil {
		solver.ui.Push(&wire.FrameT{
			Type:  wire.FrameRegulatorFrame,
			Value: regulatorWire(payload),
		})
	}

	return nil
}

/*
ObserveMark records one executable position mark for the next complete equity
revision. It never runs the optimizer directly: doing so would train many
ticker rows against the same unchanged wallet outcome and reward market-data
frequency rather than trading performance.
*/
func (solver *Solver) ObserveMark(feedback types.MarkFeedback) error {
	if solver == nil || solver.optimizer == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: initialized solver required for mark feedback",
			nil,
		))
	}

	if !feedback.Exposed {
		return nil
	}

	if feedback.Symbol == "" || feedback.At.IsZero() || feedback.Mark <= 0 ||
		math.IsNaN(feedback.Mark) || math.IsInf(feedback.Mark, 0) ||
		math.IsNaN(feedback.PeakDrawdown) || math.IsInf(feedback.PeakDrawdown, 0) ||
		feedback.PeakDrawdown > 0 || math.IsNaN(feedback.FloorDistance) ||
		math.IsInf(feedback.FloorDistance, 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: finite exposed mark feedback required",
			nil,
		))
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	if solver.marks == nil {
		solver.marks = make(map[string]observedPositionMark)
	}

	markReturn := 0.0
	previous := solver.marks[feedback.Symbol]
	samePosition := feedback.PositionID == "" || previous.positionID == feedback.PositionID
	hasPrevious := previous.value > 0 && samePosition

	if hasPrevious {
		markReturn = math.Log(feedback.Mark / previous.value)
	}

	solver.marks[feedback.Symbol] = observedPositionMark{
		positionID: feedback.PositionID,
		value:      feedback.Mark,
	}

	solver.markAcc.observe(feedback, markReturn, hasPrevious)
	solver.markSamples++
	solver.lastMarkSymbol = feedback.Symbol
	solver.lastMarkReturn = markReturn
	solver.lastMarkDrawdown = feedback.PeakDrawdown
	solver.lastMarkFloor = feedback.FloorDistance
	solver.lastMarkSurge = feedback.SurgeArmed
	solver.lastMarkAt = feedback.At

	return nil
}

/*
ObserveHindsight records one resolved opportunity attribution from the delayed
supervisory plane.
*/
func (solver *Solver) ObserveHindsight(feedback types.HindsightFeedback) error {
	if solver == nil || solver.optimizer == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: initialized solver required for hindsight feedback",
			nil,
		))
	}

	if feedback.At.IsZero() || feedback.Symbol == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: symbol and timestamp required for hindsight feedback",
			nil,
		))
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	solver.hindsightAcc.observe(feedback)
	solver.hindsightSamples++

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

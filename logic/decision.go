package logic

import (
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
Decision owns the trading decision ladder.

Normal signal measurements are retained by symbol and projected into this
sequence:

	measurements -> physical boundary deposits -> resonance -> Pearl -> action
*/
type Decision struct {
	states       map[string]*decisionState
	loop         *decisionLoop
	loopOnce     sync.Once
	loopErr      error
	gate         *decisionGate
	recorder     *audit.Recorder
	traceEnabled bool
	tick         int64
	priors       map[string]DecisionPrior
	baseFraction float64
	maxAge       time.Duration
	integration  time.Duration
}

type decisionObservation struct {
	action    *Action
	manifold  *ManifoldFrame
	resonance *ResonanceFrame
	causal    *CausalFrame
}

func NewDecision(recorder *audit.Recorder) *Decision {
	return &Decision{
		states:       map[string]*decisionState{},
		gate:         newDecisionGate(),
		recorder:     recorder,
		traceEnabled: viper.GetBool("system.audit.decisions"),
		priors:       map[string]DecisionPrior{},
		baseFraction: viper.GetFloat64("trading.sizing.base_fraction"),
		maxAge:       viper.GetDuration("market.story.measurement_max_age"),
		integration:  viper.GetDuration("telemetry.gauge.publish_interval"),
	}
}

func (decision *Decision) Close() {
	if decision == nil {
		return
	}

	if decision.loop != nil {
		decision.loop.Close()
		decision.loop = nil
	}

	if decision.recorder != nil {
		if err := decision.recorder.Close(); err != nil {
			errnie.Error(err)
		}

		decision.recorder = nil
	}
}

func (decision *Decision) Measure(
	measurements []*types.Measurement,
) (Batch, error) {
	if decision == nil || len(measurements) == 0 {
		return Batch{}, nil
	}

	decision.tick++
	batch := Batch{
		Actions:   make([]*Action, 0),
		Manifold:  make([]*ManifoldFrame, 0),
		Resonance: make([]*ResonanceFrame, 0),
		Causal:    make([]*CausalFrame, 0),
	}

	for _, measurement := range measurements {
		observation, err := decision.observe(decision.tick, measurement)

		if err != nil {
			return Batch{}, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))
		}

		if observation.manifold != nil {
			batch.Manifold = append(batch.Manifold, observation.manifold)
		}

		if observation.resonance != nil {
			batch.Resonance = append(batch.Resonance, observation.resonance)
		}

		if observation.causal != nil {
			batch.Causal = append(batch.Causal, observation.causal)
		}

		action := observation.action
		if action == nil || action.Verdict != "allow" {
			continue
		}

		if action.Fraction <= 0 || action.Price <= 0 {
			continue
		}

		batch.Actions = append(batch.Actions, action)
	}

	return batch, nil
}

func (decision *Decision) Observe(
	tick int64,
	measurement *types.Measurement,
) (*Action, error) {
	observation, err := decision.observe(tick, measurement)
	if err != nil {
		return nil, err
	}

	return observation.action, nil
}

func (decision *Decision) observe(
	tick int64,
	measurement *types.Measurement,
) (decisionObservation, error) {
	if decision == nil || measurement == nil {
		return decisionObservation{}, nil
	}

	symbol := strings.TrimSpace(measurement.Symbol)
	if symbol == "" {
		return decisionObservation{}, errnie.Err(
			errnie.Validation,
			"decision: measurement symbol required",
			nil,
		)
	}

	if decision.cascadeSource(measurement.Source) {
		return decisionObservation{}, errnie.Err(
			errnie.Validation,
			"decision: staged sources are owned by the decision ladder",
			nil,
		)
	}

	category := bestCategory(measurement.Categories)
	if category.Type == types.CategoryTypeNone {
		decision.record(stageTrace(tick, symbol, measurement.Source, "no_category"))

		return decisionObservation{}, nil
	}

	if err := decision.validate(measurement); err != nil {
		return decisionObservation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	state := decision.state(symbol)

	if state.staleSource(measurement) {
		decision.record(stageTrace(tick, symbol, measurement.Source, "stale_source"))

		return decisionObservation{}, nil
	}

	state.measurements[measurement.Source] = measurement

	if decision.stale(measurement.At, state.measurements) {
		decision.record(stageTrace(tick, symbol, measurement.Source, "stale_batch"))

		return decisionObservation{}, nil
	}

	runtime, err := decision.runtime(tick, symbol, state)

	if err != nil {
		return decisionObservation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	evaluation, err := decision.evaluate(symbol, state.measurements, runtime)

	if err != nil {
		return decisionObservation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !evaluation.ready {
		decision.record(warmupTrace(tick, symbol, measurement.Source, evaluation))

		return decisionObservation{
			manifold:  evaluation.manifold,
			resonance: evaluation.resonance,
			causal:    evaluation.causal,
		}, nil
	}

	action := decision.action(tick, symbol, evaluation.evidence)
	decision.record(verdictTrace(tick, symbol, measurement.Source, action, evaluation.evidence))

	return decisionObservation{
		action:    action,
		manifold:  evaluation.manifold,
		resonance: evaluation.resonance,
		causal:    evaluation.causal,
	}, nil
}

func (decision *Decision) evaluate(
	symbol string,
	measurements map[types.SourceType]*types.Measurement,
	runtime decisionRuntime,
) (decisionEvaluation, error) {
	decision.loopOnce.Do(func() {
		loop, err := newDecisionLoop()

		if err != nil {
			decision.loopErr = errnie.Error(errnie.Err(
				errnie.Internal,
				err.Error(),
				err,
			))

			return
		}

		decision.loop = loop
	})

	if decision.loopErr != nil {
		return decisionEvaluation{}, decision.loopErr
	}

	return decision.loop.Evaluate(symbol, measurements, runtime)
}

func (decision *Decision) state(symbol string) *decisionState {
	state := decision.states[symbol]
	if state != nil {
		return state
	}

	state = newDecisionState()
	decision.states[symbol] = state

	return state
}

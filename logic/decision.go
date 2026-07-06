package logic

import (
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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
	tick         int64
	baseFraction float64
	maxAge       time.Duration
}

type decisionState struct {
	measurements map[types.SourceType]*types.Measurement
}

func NewDecision() *Decision {
	return &Decision{
		states:       map[string]*decisionState{},
		gate:         newDecisionGate(),
		baseFraction: viper.GetFloat64("trading.sizing.base_fraction"),
		maxAge:       viper.GetDuration("market.story.measurement_max_age"),
	}
}

func (decision *Decision) Close() {
	if decision == nil || decision.loop == nil {
		return
	}

	decision.loop.Close()
	decision.loop = nil
}

func (decision *Decision) Measure(
	measurements []*types.Measurement,
) ([]*Action, error) {
	if decision == nil || len(measurements) == 0 {
		return nil, nil
	}

	decision.tick++
	actions := make([]*Action, 0)

	for _, measurement := range measurements {
		action, err := decision.Observe(decision.tick, measurement)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))
		}

		if action == nil || action.Verdict != "allow" {
			continue
		}

		if action.Fraction <= 0 || action.Price <= 0 {
			continue
		}

		actions = append(actions, action)
	}

	return actions, nil
}

func (decision *Decision) Observe(
	tick int64,
	measurement *types.Measurement,
) (*Action, error) {
	if decision == nil || measurement == nil {
		return nil, nil
	}

	symbol := strings.TrimSpace(measurement.Symbol)
	if symbol == "" {
		return nil, errnie.Err(
			errnie.Validation,
			"decision: measurement symbol required",
			nil,
		)
	}

	if decision.cascadeSource(measurement.Source) {
		return nil, errnie.Err(
			errnie.Validation,
			"decision: staged sources are owned by the decision ladder",
			nil,
		)
	}

	category := bestCategory(measurement.Categories)
	if category.Type == types.CategoryTypeNone {
		return nil, nil
	}

	if err := decision.validate(measurement); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	state := decision.state(symbol)
	state.measurements[measurement.Source] = measurement

	if decision.stale(measurement.At, state.measurements) {
		return nil, nil
	}

	evidence, ready, err := decision.evidence(symbol, state.measurements)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	return decision.action(tick, symbol, evidence), nil
}

func (decision *Decision) evidence(
	symbol string,
	measurements map[types.SourceType]*types.Measurement,
) (decisionEvidence, bool, error) {
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
		return decisionEvidence{}, false, decision.loopErr
	}

	return decision.loop.Evaluate(symbol, measurements)
}

func (decision *Decision) state(symbol string) *decisionState {
	state := decision.states[symbol]
	if state != nil {
		return state
	}

	state = &decisionState{
		measurements: map[types.SourceType]*types.Measurement{},
	}
	decision.states[symbol] = state

	return state
}

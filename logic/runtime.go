package logic

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

type DecisionPrior struct {
	TopdownPhaseScale  float64
	TopdownEnergyScale float64
}

type decisionState struct {
	measurements map[types.SourceType]*types.Measurement
	clock        *structure.ClockRing[int64]
	lastEventAt  time.Time
}

type decisionRuntime struct {
	tick        int64
	integration time.Duration
	state       *decisionState
	DeltaT      time.Duration
	Prior       DecisionPrior
}

func newDecisionState() *decisionState {
	return &decisionState{
		measurements: map[types.SourceType]*types.Measurement{},
		clock: structure.NewClockRing[int64](
			len(boundarySourceOrder),
			len(boundarySourceOrder),
			len(boundarySourceOrder),
		),
	}
}

func (decision *Decision) SetPrior(
	symbol string,
	prior DecisionPrior,
) error {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: prior symbol required",
			nil,
		))
	}

	if err := prior.validate(); err != nil {
		return err
	}

	decision.priors[symbol] = prior

	return nil
}

func (decision *Decision) runtime(
	tick int64,
	symbol string,
	state *decisionState,
) (decisionRuntime, error) {
	if decision == nil || state == nil || state.clock == nil {
		return decisionRuntime{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock is not initialized",
			nil,
		))
	}

	if decision.integration <= 0 {
		return decisionRuntime{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: integration interval required",
			nil,
		))
	}

	return decisionRuntime{
		tick:        tick,
		integration: decision.integration,
		state:       state,
		Prior:       decision.prior(symbol),
	}, nil
}

func (state *decisionState) staleSource(measurement *types.Measurement) bool {
	if state == nil || measurement == nil {
		return true
	}

	current := state.measurements[measurement.Source]
	if current == nil || current.At.IsZero() {
		return false
	}

	return measurement.At.Before(current.At)
}

func (runtime *decisionRuntime) Advance(eventAt time.Time) error {
	if runtime == nil || runtime.state == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock is not initialized",
			nil,
		))
	}

	delta, err := runtime.state.advanceClock(
		runtime.tick,
		runtime.integration,
		eventAt,
	)

	if err != nil {
		return err
	}

	runtime.DeltaT = delta

	return nil
}

func (state *decisionState) advanceClock(
	tick int64,
	integration time.Duration,
	eventAt time.Time,
) (time.Duration, error) {
	if state == nil || state.clock == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock is not initialized",
			nil,
		))
	}

	if eventAt.IsZero() {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock requires measurement time",
			nil,
		))
	}

	if integration <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: integration interval required",
			nil,
		))
	}

	if !state.lastEventAt.IsZero() && eventAt.Before(state.lastEventAt) {
		return integration, nil
	}

	delta := integration

	if !state.lastEventAt.IsZero() && eventAt.After(state.lastEventAt) {
		elapsed := eventAt.Sub(state.lastEventAt)

		if elapsed < integration {
			delta = elapsed
		}

		virtualClicks := int(elapsed/integration) - 1

		if virtualClicks > 0 {
			if _, err := state.clock.AdvanceVirtual(virtualClicks); err != nil {
				return 0, errnie.Error(errnie.Err(
					errnie.Validation,
					err.Error(),
					err,
				))
			}
		}
	}

	if _, err := state.clock.ObserveSecond(eventAt, tick); err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	state.lastEventAt = eventAt

	return delta, nil
}

func (runtime decisionRuntime) controls() (pmanifold.RuntimeControls, error) {
	if runtime.DeltaT <= 0 {
		return pmanifold.RuntimeControls{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: runtime delta must be positive",
			nil,
		))
	}

	seconds := runtime.DeltaT.Seconds()
	controls := pmanifold.RuntimeControls{
		DeltaT:             seconds,
		MetabolicRate:      1 / seconds,
		TopdownPhaseScale:  runtime.Prior.TopdownPhaseScale,
		TopdownEnergyScale: runtime.Prior.TopdownEnergyScale,
	}

	if err := controls.Validate(); err != nil {
		return pmanifold.RuntimeControls{}, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	return controls, nil
}

func (decision *Decision) prior(symbol string) DecisionPrior {
	if decision == nil {
		return DecisionPrior{}
	}

	return decision.priors[strings.TrimSpace(symbol)]
}

func (prior DecisionPrior) validate() error {
	values := map[string]float64{
		"topdown_phase_scale":  prior.TopdownPhaseScale,
		"topdown_energy_scale": prior.TopdownEnergyScale,
	}

	for name, value := range values {
		if !finite(value) {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("decision: prior %s must be finite", name),
				nil,
			))
		}

		if value < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("decision: prior %s must be non-negative", name),
				nil,
			))
		}
	}

	return nil
}

package logic

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

type DecisionPrior struct {
	TopdownPhaseScale  float64
	TopdownEnergyScale float64
}

type decisionRuntime struct {
	DeltaT time.Duration
	Prior  DecisionPrior
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
	eventAt time.Time,
) (decisionRuntime, error) {
	if decision == nil || decision.clock == nil {
		return decisionRuntime{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock is not initialized",
			nil,
		))
	}

	if eventAt.IsZero() {
		return decisionRuntime{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock requires measurement time",
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

	delta := decision.delta(eventAt)

	if err := decision.advanceClock(tick, eventAt); err != nil {
		return decisionRuntime{}, err
	}

	return decisionRuntime{
		DeltaT: delta,
		Prior:  decision.prior(symbol),
	}, nil
}

func (decision *Decision) delta(eventAt time.Time) time.Duration {
	if decision.lastEventAt.IsZero() || !eventAt.After(decision.lastEventAt) {
		return decision.integration
	}

	elapsed := eventAt.Sub(decision.lastEventAt)

	if elapsed < decision.integration {
		return elapsed
	}

	return decision.integration
}

func (decision *Decision) advanceClock(tick int64, eventAt time.Time) error {
	if !decision.lastEventAt.IsZero() && eventAt.Before(decision.lastEventAt) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision: event clock received out-of-order measurement time",
			nil,
		))
	}

	if !decision.lastEventAt.IsZero() && eventAt.After(decision.lastEventAt) {
		elapsed := eventAt.Sub(decision.lastEventAt)
		virtualClicks := int(elapsed/decision.integration) - 1

		if virtualClicks > 0 {
			if _, err := decision.clock.AdvanceVirtual(virtualClicks); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					err.Error(),
					err,
				))
			}
		}
	}

	if _, err := decision.clock.ObserveSecond(eventAt, tick); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	decision.lastEventAt = eventAt

	return nil
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

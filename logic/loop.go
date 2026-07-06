package logic

import (
	"errors"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type decisionLoop struct {
	boundaries     *boundaryClamps
	physical       *physicalManifold
	predictive     *cognitiveManifold
	counterfactual *causalCounterfactual
}

func newDecisionLoop() (*decisionLoop, error) {
	physical, err := newPhysicalManifold()
	if err != nil {
		return nil, err
	}

	predictive, err := newCognitiveManifold()
	if err != nil {
		physical.Close()
		return nil, err
	}

	return &decisionLoop{
		boundaries:     newBoundaryClamps(),
		physical:       physical,
		predictive:     predictive,
		counterfactual: newCausalCounterfactual(),
	}, nil
}

func (loop *decisionLoop) Close() {
	if loop == nil {
		return
	}

	loop.physical.Close()
	loop.predictive.Close()
}

func (loop *decisionLoop) Evaluate(
	symbol string,
	measurements map[types.SourceType]*types.Measurement,
) (decisionEvidence, bool, error) {
	frame, err := loop.boundaries.Frame(symbol, measurements)
	if err != nil {
		if errors.Is(err, errBoundaryNoClamps) {
			return decisionEvidence{}, false, nil
		}

		return decisionEvidence{}, false, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if frame.price <= 0 {
		return decisionEvidence{}, false, nil
	}

	physical, err := loop.physical.Settle(frame)
	if err != nil {
		return decisionEvidence{}, false, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	predictive, err := loop.predictive.Settle(physical)
	if err != nil {
		return decisionEvidence{}, false, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	intervened, err := loop.physical.Settle(frame.Intervene())
	if err != nil {
		return decisionEvidence{}, false, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	counterfactual, ready, err := loop.counterfactual.Evaluate(
		symbol,
		frame,
		physical,
		intervened,
		predictive,
	)
	if err != nil || !ready {
		return decisionEvidence{}, ready, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	decisionAt := ""
	if !frame.eventAt.IsZero() {
		decisionAt = frame.eventAt.UTC().Format(time.RFC3339Nano)
	}

	return decisionEvidence{
		physical:       physical,
		predictive:     predictive,
		counterfactual: counterfactual,
		price:          frame.price,
		momentum:       frame.netMomentum(),
		pressure:       frame.netPressure(),
		at:             decisionAt,
	}, true, nil
}

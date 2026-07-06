package logic

import (
	"errors"

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
) (decisionEvaluation, error) {
	frame, err := loop.boundaries.Frame(symbol, measurements)
	if err != nil {
		if errors.Is(err, errBoundaryNoClamps) {
			return decisionEvaluation{}, nil
		}

		return decisionEvaluation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if frame.price <= 0 {
		return decisionEvaluation{}, nil
	}

	physical, err := loop.physical.Settle(frame)
	if err != nil {
		return decisionEvaluation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	manifold := frame.manifold(ManifoldGrid{
		X: loop.physical.config.GridX,
		Y: loop.physical.config.GridY,
		Z: loop.physical.config.GridZ,
	}, physical)

	predictive, err := loop.predictive.Settle(physical)
	if err != nil {
		return decisionEvaluation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	resonance := frame.resonance(predictive)

	intervened, err := loop.physical.Settle(frame.Intervene())
	if err != nil {
		return decisionEvaluation{}, errnie.Error(errnie.Err(
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

	if err != nil {
		return decisionEvaluation{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return decisionEvaluation{
			manifold:  manifold,
			resonance: resonance,
		}, nil
	}

	causal := frame.causal(counterfactual)

	return decisionEvaluation{
		evidence: decisionEvidence{
			physical:       physical,
			predictive:     predictive,
			counterfactual: counterfactual,
			price:          frame.price,
			momentum:       frame.netMomentum(),
			pressure:       frame.netPressure(),
			at:             frame.at(),
		},
		ready:     true,
		manifold:  manifold,
		resonance: resonance,
		causal:    causal,
	}, nil
}

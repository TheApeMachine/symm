package regulator

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/system"
	"gonum.org/v1/gonum/stat/distuv"
)

func (optimizer *optimizer) selectControls(
	context []float64,
	active bool,
) (controlVector, bool, float64, bool, error) {
	skill, skillReady := optimizer.returnCoder.Manifold().TaskSkill()

	if !active {
		selected := optimizer.current
		selected[controlAllocation] = 1
		selected[controlThesisScore] = 0
		selected[controlConfidence] = 0
		selected[controlSupport] = 0
		selected[controlContradiction] = 1
		selected[controlGraphThreshold] = 0

		return selected, selected != optimizer.current, skill, skillReady, nil
	}

	if optimizer.resolved == 0 {
		return optimizer.current, false, skill, skillReady, nil
	}

	movable := optimizer.space.movable()
	persistent := len(movable) > 0 && optimizer.resolved%len(movable) == 0

	if skillReady && skill > 1 && !persistent {
		selected, fallback, err := optimizer.best(context)

		if !fallback || err != nil {
			return selected, fallback, skill, skillReady, err
		}
	}

	selected := optimizer.space.exploratory(
		optimizer.current,
		optimizer.resolved,
		optimizer.interventions,
	)
	optimizer.interventions++

	return selected, true, skill, skillReady, nil
}

func (optimizer *optimizer) best(
	context []float64,
) (controlVector, bool, error) {
	candidates := optimizer.space.candidates(
		optimizer.current,
		optimizer.resolved,
	)
	best := optimizer.current
	bestScore := candidateScore{
		losing:        true,
		inactive:      true,
		returnFloor:   math.Inf(-1),
		activityFloor: math.Inf(-1),
	}

	for _, candidate := range candidates {
		candidateInput := optimizer.input(context, candidate)

		if _, err := optimizer.returnCoder.Manifold().SettleFromBatchOptions(
			candidateInput,
			nil,
			false,
			false,
		); err != nil {
			return controlVector{}, false, fmt.Errorf(
				"regulator: settle candidate return control state: %w",
				err,
			)
		}

		if _, err := optimizer.activityCoder.Manifold().SettleFromBatchOptions(
			candidateInput,
			nil,
			false,
			false,
		); err != nil {
			return controlVector{}, false, fmt.Errorf(
				"regulator: settle candidate activity control state: %w",
				err,
			)
		}

		forecast, activity, ready, err := optimizer.forecast()

		if err != nil {
			return controlVector{}, false, err
		}

		if !ready {
			return optimizer.space.exploratory(
				optimizer.current,
				optimizer.resolved,
				optimizer.interventions,
			), true, nil
		}

		returnDistribution := distuv.StudentsT{
			Mu:    forecast.Value,
			Sigma: forecast.Scale,
			Nu:    forecast.DegreesOfFreedom,
		}
		activityDistribution := distuv.StudentsT{
			Mu:    activity.Value,
			Sigma: activity.Scale,
			Nu:    activity.DegreesOfFreedom,
		}
		score := candidateScore{
			losing: returnDistribution.Quantile(optimizer.confidence) < 0,
			inactive: activityDistribution.Quantile(
				optimizer.confidence,
			) < system.UninformativeDirectionConfidence,
			returnFloor: returnDistribution.Quantile(1 - optimizer.confidence),
			activityFloor: activityDistribution.Quantile(
				1 - optimizer.confidence,
			),
		}

		if score.Better(bestScore) {
			best = candidate
			bestScore = score
		}
	}

	return best, false, nil
}

func (optimizer *optimizer) input(
	context []float64,
	controls controlVector,
) []float64 {
	input := make([]float64, 0, regulatorContextCount+controlCount)
	input = append(input, context...)
	input = append(input, controls[:]...)

	return input
}

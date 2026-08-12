package regulator

import (
	"fmt"
	"math"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"gonum.org/v1/gonum/stat/distuv"
)

const regulatorContextCount = 4

type optimizationResult struct {
	controls      controlVector
	forecast      learning.RLSOutput
	forecastReady bool
	skill         float64
	skillReady    bool
	surprise      float64
	energy        float64
	exploring     bool
}

/*
optimizer learns the temporal response from one applied control vector to the
next account log return and selects the next bounded intervention.
*/
type optimizer struct {
	coder         *learning.ResonanceManifold
	space         *controlSpace
	returnScale   *adaptive.Standardizer
	drawdownScale *adaptive.Standardizer
	confidence    float64
	baseline      controlVector
	current       controlVector
	pending       []float64
	resolved      int
	interventions int
}

func newOptimizer(config *system.Config) (*optimizer, error) {
	space, err := newControlSpace(config)

	if err != nil {
		return nil, err
	}

	if config.Resonance == nil || config.Resonance.LearningRate <= 0 {
		return nil, fmt.Errorf("regulator: positive predictive-coding pace required")
	}

	if config.Regulator == nil || config.Regulator.OptimizationConfidence <= 0.5 ||
		config.Regulator.OptimizationConfidence >= 1 {
		return nil, fmt.Errorf("regulator: optimization confidence must be in (0.5,1)")
	}

	inputCount := regulatorContextCount + controlCount
	architecture := []int{inputCount, inputCount + controlCount, inputCount}
	coder := learning.NewResonanceManifold(
		architecture,
		1,
		config.Resonance.LearningRate,
	)

	if coder == nil {
		return nil, fmt.Errorf("regulator: predictive-coding manifold construction failed")
	}

	initial := space.current(config)

	return &optimizer{
		coder:         coder,
		space:         space,
		returnScale:   adaptive.NewStandardizer(),
		drawdownScale: adaptive.NewStandardizer(),
		confidence:    config.Regulator.OptimizationConfidence,
		baseline:      initial,
		current:       initial,
	}, nil
}

func (optimizer *optimizer) update(
	periodReturn float64,
	drawdown float64,
) (optimizationResult, error) {
	if err := optimizer.resolve(periodReturn); err != nil {
		return optimizationResult{}, err
	}

	context, err := optimizer.context(periodReturn, drawdown)

	if err != nil {
		return optimizationResult{}, err
	}

	selected, exploring, skill, skillReady, err := optimizer.selectControls(context)

	if err != nil {
		return optimizationResult{}, err
	}

	input := optimizer.input(context, selected)

	if _, err := optimizer.coder.SettleFromBatchOptions(
		input,
		nil,
		false,
		// Supervised resolution advances the retained temporal state. Advancing
		// here would replace the real prior with the candidate before it occurs.
		false,
	); err != nil {
		return optimizationResult{}, fmt.Errorf(
			"regulator: settle selected control state: %w",
			err,
		)
	}

	forecast, forecastReady, err := optimizer.forecast()

	if err != nil {
		return optimizationResult{}, err
	}

	result := optimizationResult{
		controls:      selected,
		forecast:      forecast,
		forecastReady: forecastReady,
		skill:         skill,
		skillReady:    skillReady,
		surprise:      optimizer.coder.ReconstructionError(),
		energy:        optimizer.coder.Energy(),
		exploring:     exploring,
	}

	optimizer.current = selected
	optimizer.pending = input

	return result, nil
}

func (optimizer *optimizer) resolve(periodReturn float64) error {
	if optimizer.pending == nil {
		return nil
	}

	if _, err := optimizer.coder.SettleFromBatchOptions(
		optimizer.pending,
		[]float64{periodReturn},
		true,
		false,
	); err != nil {
		return fmt.Errorf("regulator: resolve prior control outcome: %w", err)
	}

	optimizer.resolved++
	return nil
}

func (optimizer *optimizer) selectControls(
	context []float64,
) (controlVector, bool, float64, bool, error) {
	skill, skillReady := optimizer.coder.TaskSkill()

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
	bestScore := math.Inf(-1)

	for _, candidate := range candidates {
		if _, err := optimizer.coder.SettleFromBatchOptions(
			optimizer.input(context, candidate),
			nil,
			false,
			false,
		); err != nil {
			return controlVector{}, false, fmt.Errorf(
				"regulator: settle candidate control state: %w",
				err,
			)
		}

		forecast, ready, err := optimizer.forecast()

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

		distribution := distuv.StudentsT{
			Mu:    forecast.Value,
			Sigma: forecast.Scale,
			Nu:    forecast.DegreesOfFreedom,
		}
		score := distribution.Quantile(optimizer.confidence)

		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}

	return best, false, nil
}

func (optimizer *optimizer) context(
	periodReturn float64,
	drawdown float64,
) ([]float64, error) {
	returnOutput, err := optimizer.returnScale.Measure(periodReturn)

	if err != nil {
		return nil, fmt.Errorf("regulator: standardize account return: %w", err)
	}

	drawdownOutput, err := optimizer.drawdownScale.Measure(drawdown)

	if err != nil {
		return nil, fmt.Errorf("regulator: standardize account drawdown: %w", err)
	}

	return []float64{
		returnOutput.Value,
		readiness(returnOutput.Ready),
		drawdownOutput.Value,
		readiness(drawdownOutput.Ready),
	}, nil
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

func (optimizer *optimizer) forecast() (
	learning.RLSOutput,
	bool,
	error,
) {
	forecasts, err := optimizer.coder.RolloutTaskForecast(1)

	if err != nil {
		return learning.RLSOutput{}, false, fmt.Errorf(
			"regulator: forecast candidate account return: %w",
			err,
		)
	}

	if len(forecasts) == 0 || !forecasts[0].Ready {
		return learning.RLSOutput{}, false, nil
	}

	return forecasts[0], true, nil
}

func readiness(ready bool) float64 {
	if ready {
		return 1
	}

	return 0
}

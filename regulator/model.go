package regulator

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
)

const regulatorContextCount = 17

type optimizationResult struct {
	controls      controlVector
	forecast      learning.RLSOutput
	activity      learning.RLSOutput
	forecastReady bool
	activityReady bool
	skill         float64
	skillReady    bool
	surprise      float64
	energy        float64
	exploring     bool
}

/*
candidateScore preserves the regulator's ordered objective without blending
loss, inactivity, and return into arbitrary weights.
*/
type candidateScore struct {
	losing        bool
	inactive      bool
	returnFloor   float64
	activityFloor float64
}

func (score candidateScore) Better(incumbent candidateScore) bool {
	if score.losing != incumbent.losing {
		return !score.losing
	}

	if score.inactive != incumbent.inactive {
		return !score.inactive
	}

	if score.returnFloor != incumbent.returnFloor {
		return score.returnFloor > incumbent.returnFloor
	}

	return score.activityFloor > incumbent.activityFloor
}

/*
optimizer learns the temporal response from one applied control vector to the
next account log return and selects the next bounded intervention.
*/
type optimizer struct {
	coder         *learning.PredictiveCoder
	space         *controlSpace
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
	arch := []int{inputCount, inputCount + controlCount, inputCount}

	coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
		CustomArch: arch,
		TargetDim:  2,
		MaxHorizon: 1,
		Pace:       config.Resonance.LearningRate,
		Learn:      true,
	})

	if coder == nil {
		return nil, fmt.Errorf("regulator: predictive coder construction failed")
	}

	initial := space.current(config)

	return &optimizer{
		coder:      coder,
		space:      space,
		confidence: config.Regulator.OptimizationConfidence,
		baseline:   initial,
		current:    initial,
	}, nil
}

func (optimizer *optimizer) update(
	periodReturn float64,
	drawdown float64,
	active bool,
	marks markContext,
	hindsight hindsightContext,
) (optimizationResult, error) {
	if err := optimizer.resolve(periodReturn, active); err != nil {
		return optimizationResult{}, err
	}

	context := regulatorContext(periodReturn, drawdown, active, marks, hindsight)

	selected, exploring, skill, skillReady, err := optimizer.selectControls(
		context,
		active,
	)

	if err != nil {
		return optimizationResult{}, err
	}

	input := optimizer.input(context, selected)
	forecasts, err := optimizer.coder.Evaluate(input)

	if err != nil {
		return optimizationResult{}, fmt.Errorf("regulator: evaluate selected controls: %w", err)
	}

	forecast, activity, forecastReady := optimizer.extractForecast(forecasts)

	result := optimizationResult{
		controls:      selected,
		forecast:      forecast,
		activity:      activity,
		forecastReady: forecastReady,
		activityReady: forecastReady,
		skill:         skill,
		skillReady:    skillReady,
		surprise:      optimizer.coder.Manifold().ReconstructionError(),
		energy:        optimizer.coder.Manifold().Energy(),
		exploring:     exploring,
	}

	optimizer.current = selected
	optimizer.pending = input

	return result, nil
}

func (optimizer *optimizer) resolve(periodReturn float64, active bool) error {
	if optimizer.pending == nil {
		return nil
	}

	activeOutcome := 0.0

	if active {
		activeOutcome = 1.0
	}

	if err := optimizer.coder.ResolveTargets(
		optimizer.pending,
		[]float64{periodReturn, activeOutcome},
	); err != nil {
		return fmt.Errorf("regulator: resolve prior outcomes: %w", err)
	}

	optimizer.resolved++

	return nil
}

func (optimizer *optimizer) extractForecast(
	forecasts []learning.RLSOutput,
) (learning.RLSOutput, learning.RLSOutput, bool) {
	if len(forecasts) < 2 || !forecasts[0].Ready || !forecasts[1].Ready {
		return learning.RLSOutput{}, learning.RLSOutput{}, false
	}

	return forecasts[0], forecasts[1], true
}

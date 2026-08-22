package regulator

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
)

const regulatorContextCount = 10

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
	returnCoder   *learning.PredictiveCoder
	activityCoder *learning.PredictiveCoder
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

	returnCoder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
		CustomArch: arch,
		Target:     learning.IdentityTarget(),
		MaxHorizon: 1,
		Pace:       config.Resonance.LearningRate,
		Learn:      true,
	})

	if returnCoder == nil {
		return nil, fmt.Errorf("regulator: return predictive coder construction failed")
	}

	activityCoder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
		CustomArch: arch,
		Target:     learning.IdentityTarget(),
		MaxHorizon: 1,
		Pace:       config.Resonance.LearningRate,
		Learn:      true,
	})

	if activityCoder == nil {
		return nil, fmt.Errorf("regulator: activity predictive coder construction failed")
	}

	initial := space.current(config)

	return &optimizer{
		returnCoder:   returnCoder,
		activityCoder: activityCoder,
		space:         space,
		confidence:    config.Regulator.OptimizationConfidence,
		baseline:      initial,
		current:       initial,
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

	if _, err := optimizer.returnCoder.Manifold().SettleFromBatchOptions(
		input,
		nil,
		false,
		false,
	); err != nil {
		return optimizationResult{}, fmt.Errorf(
			"regulator: settle selected return control state: %w",
			err,
		)
	}

	if _, err := optimizer.activityCoder.Manifold().SettleFromBatchOptions(
		input,
		nil,
		false,
		false,
	); err != nil {
		return optimizationResult{}, fmt.Errorf(
			"regulator: settle selected activity control state: %w",
			err,
		)
	}

	forecast, activity, forecastReady, err := optimizer.forecast()

	if err != nil {
		return optimizationResult{}, err
	}

	result := optimizationResult{
		controls:      selected,
		forecast:      forecast,
		activity:      activity,
		forecastReady: forecastReady,
		activityReady: forecastReady,
		skill:         skill,
		skillReady:    skillReady,
		surprise:      optimizer.returnCoder.Manifold().ReconstructionError(),
		energy:        optimizer.returnCoder.Manifold().Energy(),
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

	if _, err := optimizer.returnCoder.Manifold().SettleFromBatchOptions(
		optimizer.pending,
		[]float64{periodReturn},
		true,
		false,
	); err != nil {
		return fmt.Errorf("regulator: resolve prior return outcome: %w", err)
	}

	if _, err := optimizer.activityCoder.Manifold().SettleFromBatchOptions(
		optimizer.pending,
		[]float64{activeOutcome},
		true,
		false,
	); err != nil {
		return fmt.Errorf("regulator: resolve prior activity outcome: %w", err)
	}

	optimizer.resolved++

	return nil
}

func (optimizer *optimizer) forecast() (
	learning.RLSOutput,
	learning.RLSOutput,
	bool,
	error,
) {
	returnForecasts, err := optimizer.returnCoder.Manifold().RolloutTaskForecast(1)

	if err != nil {
		return learning.RLSOutput{}, learning.RLSOutput{}, false, fmt.Errorf(
			"regulator: forecast candidate account return: %w",
			err,
		)
	}

	activityForecasts, err := optimizer.activityCoder.Manifold().RolloutTaskForecast(1)

	if err != nil {
		return learning.RLSOutput{}, learning.RLSOutput{}, false, fmt.Errorf(
			"regulator: forecast candidate account activity: %w",
			err,
		)
	}

	if len(returnForecasts) == 0 || !returnForecasts[0].Ready ||
		len(activityForecasts) == 0 || !activityForecasts[0].Ready {
		return learning.RLSOutput{}, learning.RLSOutput{}, false, nil
	}

	return returnForecasts[0], activityForecasts[0], true, nil
}

package regulator

import (
	"fmt"
	"math"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"gonum.org/v1/gonum/stat/distuv"
)

const regulatorContextCount = 13

const (
	targetReturn = iota
	targetActivity
	targetCount
)

/*
markContext summarizes all executable position marks observed since the prior
account valuation. Dense marks condition the next control decision, but the
complete broker equity revision remains the supervised wallet outcome.
*/
type markContext struct {
	samples       int
	returnSamples int
	meanReturn    float64
	worstDrawdown float64
	minimumFloor  float64
	surgeFraction float64
}

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

/*
Better reports whether this outcome is preferable under the regulator policy:
avoid a confidently losing wallet first, avoid confident inactivity second,
then maximize conservative wallet return and activity evidence.
*/
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
	coder             *learning.ResonanceManifold
	space             *controlSpace
	returnScale       *adaptive.Standardizer
	drawdownScale     *adaptive.Standardizer
	markReturnScale   *adaptive.Standardizer
	markDrawdownScale *adaptive.Standardizer
	markFloorScale    *adaptive.Standardizer
	confidence        float64
	baseline          controlVector
	current           controlVector
	pending           []float64
	resolved          int
	interventions     int
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
		targetCount,
		config.Resonance.LearningRate,
	)

	if coder == nil {
		return nil, fmt.Errorf("regulator: predictive-coding manifold construction failed")
	}

	initial := space.current(config)

	return &optimizer{
		coder:             coder,
		space:             space,
		returnScale:       adaptive.NewStandardizer(),
		drawdownScale:     adaptive.NewStandardizer(),
		markReturnScale:   adaptive.NewStandardizer(),
		markDrawdownScale: adaptive.NewStandardizer(),
		markFloorScale:    adaptive.NewStandardizer(),
		confidence:        config.Regulator.OptimizationConfidence,
		baseline:          initial,
		current:           initial,
	}, nil
}

func (optimizer *optimizer) update(
	periodReturn float64,
	drawdown float64,
	active bool,
	marks markContext,
) (optimizationResult, error) {
	if err := optimizer.resolve(periodReturn, active); err != nil {
		return optimizationResult{}, err
	}

	context, err := optimizer.context(periodReturn, drawdown, active, marks)

	if err != nil {
		return optimizationResult{}, err
	}

	selected, exploring, skill, skillReady, err := optimizer.selectControls(
		context,
		active,
	)

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
		surprise:      optimizer.coder.ReconstructionError(),
		energy:        optimizer.coder.Energy(),
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

	if _, err := optimizer.coder.SettleFromBatchOptions(
		optimizer.pending,
		[]float64{periodReturn, readiness(active)},
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
	active bool,
) (controlVector, bool, float64, bool, error) {
	skill, skillReady := optimizer.coder.TaskSkill()

	if !active {
		selected := optimizer.current
		selected[controlAllocation] = 1
		selected[controlConfidence] = 0
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

func (optimizer *optimizer) context(
	periodReturn float64,
	drawdown float64,
	active bool,
	marks markContext,
) ([]float64, error) {
	returnOutput, err := optimizer.returnScale.Measure(periodReturn)

	if err != nil {
		return nil, fmt.Errorf("regulator: standardize account return: %w", err)
	}

	drawdownOutput, err := optimizer.drawdownScale.Measure(drawdown)

	if err != nil {
		return nil, fmt.Errorf("regulator: standardize account drawdown: %w", err)
	}

	markReturnValue := 0.0
	markReturnReady := false

	if marks.returnSamples > 0 {
		markReturnOutput, markErr := optimizer.markReturnScale.Measure(marks.meanReturn)

		if markErr != nil {
			return nil, fmt.Errorf("regulator: standardize mark return: %w", markErr)
		}

		markReturnValue = markReturnOutput.Value
		markReturnReady = markReturnOutput.Ready
	}

	markDrawdownValue := 0.0
	markDrawdownReady := false
	markFloorValue := 0.0
	markFloorReady := false

	if marks.samples > 0 {
		markDrawdownOutput, markErr := optimizer.markDrawdownScale.Measure(
			marks.worstDrawdown,
		)

		if markErr != nil {
			return nil, fmt.Errorf("regulator: standardize mark drawdown: %w", markErr)
		}

		markDrawdownValue = markDrawdownOutput.Value
		markDrawdownReady = markDrawdownOutput.Ready

		markFloorOutput, floorErr := optimizer.markFloorScale.Measure(
			marks.minimumFloor,
		)

		if floorErr != nil {
			return nil, fmt.Errorf("regulator: standardize stop-floor distance: %w", floorErr)
		}

		markFloorValue = markFloorOutput.Value
		markFloorReady = markFloorOutput.Ready
	}

	return []float64{
		returnOutput.Value,
		readiness(returnOutput.Ready),
		drawdownOutput.Value,
		readiness(drawdownOutput.Ready),
		readiness(active),
		readiness(marks.samples > 0),
		markReturnValue,
		readiness(markReturnReady),
		markDrawdownValue,
		readiness(markDrawdownReady),
		markFloorValue,
		readiness(markFloorReady),
		marks.surgeFraction,
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
	learning.RLSOutput,
	bool,
	error,
) {
	forecasts, err := optimizer.coder.RolloutTaskForecast(1)

	if err != nil {
		return learning.RLSOutput{}, learning.RLSOutput{}, false, fmt.Errorf(
			"regulator: forecast candidate account return: %w",
			err,
		)
	}

	if len(forecasts) != targetCount || !forecasts[targetReturn].Ready ||
		!forecasts[targetActivity].Ready {
		return learning.RLSOutput{}, learning.RLSOutput{}, false, nil
	}

	return forecasts[targetReturn], forecasts[targetActivity], true, nil
}

func readiness(ready bool) float64 {
	if ready {
		return 1
	}

	return 0
}

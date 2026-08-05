package causal

import (
	"math"
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type causalInput struct {
	symbol       string
	row          []float64
	intervention float64
	contagion    float64
	condition    float64
}

type causalTarget struct {
	sum   float64
	count int
}

/*
buildCausalInputs constructs deterministic symbol-local inputs from one target
index so causal row assembly is linear in the number of measurements.
*/
func (solver *Solver) buildCausalInputs(thesis *types.Thesis) []causalInput {
	targets := solver.causalTargets(thesis)
	symbols := make([]string, 0, len(targets))

	for symbol := range targets {
		if symbol == "default" {
			continue
		}

		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	inputs := make([]causalInput, 0, len(symbols))

	for _, symbol := range symbols {
		input, ok := solver.buildCausalInput(thesis, symbol, targets[symbol])

		if ok {
			inputs = append(inputs, input)
		}
	}

	return inputs
}

func (solver *Solver) causalTargets(thesis *types.Thesis) map[string]causalTarget {
	targets := make(map[string]causalTarget)

	if thesis == nil || thesis.Measurements == nil {
		return targets
	}

	thesis.Measurements.Range(func(_, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if !ok {
			measurement, measurementOK := value.(*types.Measurement)

			if !measurementOK || measurement == nil {
				return true
			}

			rows = []*types.Measurement{measurement}
		}

		for _, measurement := range rows {
			if measurement == nil {
				continue
			}

			solver.addCausalTarget(targets, measurement)
		}

		return true
	})

	return targets
}

func (solver *Solver) addCausalTarget(
	targets map[string]causalTarget,
	measurement *types.Measurement,
) {
	for key, metric := range measurement.Metrics {
		if math.IsNaN(metric.Raw) || math.IsInf(metric.Raw, 0) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"causal: dropped non-finite measurement metric",
				nil,
			))

			continue
		}

		if !isRealizedReturnMetric(key) {
			continue
		}

		aggregate := targets["default"]
		aggregate.sum += metric.Raw
		aggregate.count++
		targets["default"] = aggregate

		if measurement.Symbol == "" {
			continue
		}

		target := targets[measurement.Symbol]
		target.sum += metric.Raw
		target.count++
		targets[measurement.Symbol] = target
	}
}

func isRealizedReturnMetric(key string) bool {
	metric, _ := types.ParseMetricKey(key)

	switch metric {
	case types.MetricChange,
		types.MetricTrend,
		types.MetricNetFraction,
		types.MetricSignedLagDirection,
		types.MetricRelativeLead:
		return true
	}

	switch key {
	case "change", "trend", "return", "price_return", "realized_return",
		"finite", "first", "second", "single":
		return true
	default:
		return false
	}
}

func (solver *Solver) buildCausalInput(
	thesis *types.Thesis,
	symbol string,
	target causalTarget,
) (causalInput, bool) {
	if target.count == 0 {
		return causalInput{}, false
	}

	resonanceRaw, found := thesis.Resonance.Load(symbol)

	if !found {
		return causalInput{}, false
	}

	resonance, ok := resonanceRaw.(types.ResonanceReading)

	if !ok || resonance.Forecast == nil ||
		resonance.ForecastValidity.State != types.ValidityValid {
		return causalInput{}, false
	}

	if err := resonance.Forecast.Validate(); err != nil {
		return causalInput{}, false
	}

	taskPrediction, predictionReady := resonance.Forecast.Step(0)
	realizedReturn := target.sum / float64(target.count)

	if !predictionReady || math.IsNaN(resonance.Energy) ||
		math.IsInf(resonance.Energy, 0) || math.IsNaN(resonance.Surprise) ||
		math.IsInf(resonance.Surprise, 0) || math.IsNaN(realizedReturn) ||
		math.IsInf(realizedReturn, 0) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: dropped invalid resonance or realized return evidence",
			nil,
		))

		return causalInput{}, false
	}

	return causalInput{
		symbol: symbol,
		row: []float64{
			resonance.Energy,
			resonance.Surprise,
			taskPrediction,
			realizedReturn,
		},
		intervention: taskPrediction,
		contagion:    resonance.Surprise,
		condition:    resonance.Energy,
	}, true
}

/*
buildCausalRow constructs an aligned 4-column causal observation vector [C1, C2, T, Y]
from Thesis measurements and predictive coding outputs.
*/
func (solver *Solver) buildCausalRow(
	thesis *types.Thesis,
	symbol string,
) (row []float64, intervention float64, contagion float64, condition float64, ok bool) {
	targets := solver.causalTargets(thesis)
	target, found := targets[symbol]

	if !found {
		return nil, 0, 0, 0, false
	}

	input, valid := solver.buildCausalInput(thesis, symbol, target)

	if !valid {
		return nil, 0, 0, 0, false
	}

	return input.row,
		input.intervention,
		input.contagion,
		input.condition,
		true
}

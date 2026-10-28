package causal

import (
	"math"
	"sort"

	"github.com/theapemachine/errnie"
)

/*
RegimeConfig describes causal regime switching over tabular node rows.
*/
type RegimeConfig struct {
	Target          int
	ContagionBreak  float64
	ContagionSkip   []int
	ConditionSwitch float64
	ConditionLeft   int
	ConditionRight  int
}

/*
RegimeInput carries retained rows plus the latest contagion measurement.
*/
type RegimeInput struct {
	Rows      [][]float64
	Contagion float64
}

/*
RegimeOutput reports the raw regime gate and condition number.
*/
type RegimeOutput struct {
	Value       float64
	Condition   float64
	RawInverted float64
}

/*
Regime selects normal or inverted DAG roles from contagion and pair condition.
*/
type Regime struct {
	config RegimeConfig
}

/*
NewRegime returns a typed regime selector.
*/
func NewRegime(config RegimeConfig) *Regime {
	return &Regime{
		config: config,
	}
}

/*
Measure evaluates the regime switch against retained node rows.
*/
func (regime *Regime) Measure(input RegimeInput) (RegimeOutput, error) {
	table, err := newNodeTable(input.Rows, regime.config.Target)
	if err != nil {
		return RegimeOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal regime: table construction failed",
			err,
		))
	}

	breakPoint := regime.config.ContagionBreak

	if breakPoint <= 0 {
		breakPoint = dynamicContagionBreak(
			input.Rows,
			regime.config.Target,
			regime.config.ContagionSkip,
		)
	}

	contagionBreak := breakPoint > 0 && input.Contagion > breakPoint
	condition := 0.0
	conditionBreak := false

	if regime.config.ConditionSwitch > 0 {
		condition, err = table.pairConditionNumber(
			regime.config.ConditionLeft,
			regime.config.ConditionRight,
		)
		if err != nil {
			return RegimeOutput{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"causal regime: pair condition failed",
				err,
			))
		}

		conditionBreak = math.IsInf(condition, 1) ||
			condition >= regime.config.ConditionSwitch
	}

	rawInverted := 0.0

	if conditionBreak || contagionBreak {
		rawInverted = 1
	}

	return RegimeOutput{
		Value:       condition,
		Condition:   condition,
		RawInverted: rawInverted,
	}, nil
}

func dynamicContagionBreak(rows [][]float64, target int, skip []int) float64 {
	samples := make([]float64, 0, len(rows))

	for _, row := range rows {
		for nodeIndex, sample := range row {
			if nodeIndex == target || containsIndex(skip, nodeIndex) {
				continue
			}

			samples = append(samples, math.Abs(sample))
		}
	}

	if len(samples) == 0 {
		return 0
	}

	center := median(samples)
	deviations := make([]float64, 0, len(samples))

	for _, sample := range samples {
		deviations = append(deviations, math.Abs(sample-center))
	}

	return center + median(deviations)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2

	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}

	return sorted[middle]
}

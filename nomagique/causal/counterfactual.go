package causal

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Counterfactual composes abduction, intervention and prediction.
*/
type Counterfactual struct {
	*core.PrimitiveError
	tolerance    float64
	intercept    float64
	coefficients []float64
	target       int
	treatment    int
	features     []int
	rows         [][]float64
	input        data.Map[string]
	output       data.Map[float64]
}

func NewCounterfactual(tolerance float64, evidence ...any) *Counterfactual {
	output := data.NewOutputMap()
	output.Values["counterfactual"] = 0
	output.Values["noise"] = 0
	output.Values["precision"] = 0
	output.Values["defined"] = 0

	op := &Counterfactual{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
		input: data.NewMap(
			"level", "level",
			"actual", "actual",
			"factual", "factual",
			"predicted", "predicted",
		),
		output: output,
	}

	if len(evidence) < 4 {
		return op
	}

	rows, rowsOK := evidence[0].([][]float64)
	target, targetOK := evidence[1].(int)
	treatment, treatmentOK := evidence[2].(int)
	features, featuresOK := evidence[3].([]int)

	if !rowsOK || !targetOK || !treatmentOK || !featuresOK {
		op.Error(core.ErrShape)
		return op
	}

	if len(rows) == 0 || target < 0 {
		op.Error(core.ErrDomain)
		return op
	}

	columnCount := len(rows[0])

	if target >= columnCount || treatment < 0 || treatment >= columnCount {
		op.Error(core.ErrShape)
		return op
	}

	validFeats := make([]int, 0, len(features)+1)
	seen := make(map[int]bool, len(features)+1)

	for _, feature := range append(slices.Clone(features), treatment) {
		if feature < 0 || feature >= columnCount {
			op.Error(core.ErrShape)
			return op
		}

		if feature == target || seen[feature] {
			continue
		}

		seen[feature] = true
		validFeats = append(validFeats, feature)
	}

	if len(validFeats) == 0 {
		op.Error(core.ErrDomain)
		return op
	}

	copied := make([][]float64, len(rows))

	for rowIndex, row := range rows {
		if len(row) != columnCount {
			op.Error(core.ErrShape)
			return op
		}

		copied[rowIndex] = slices.Clone(row)
	}

	design := mat.NewDense(len(copied), len(validFeats)+1, nil)
	outcome := mat.NewDense(len(copied), 1, nil)

	for rowIndex, row := range copied {
		design.Set(rowIndex, 0, 1)

		for featureIndex, column := range validFeats {
			design.Set(rowIndex, featureIndex+1, row[column])
		}

		outcome.Set(rowIndex, 0, row[target])
	}

	var solved mat.Dense
	err := solved.Solve(design, outcome)

	if err != nil {
		op.Error(core.ErrDomain)
		return op
	}

	op.rows = copied
	op.target = target
	op.treatment = treatment
	op.features = validFeats
	op.intercept = solved.At(0, 0)
	op.coefficients = make([]float64, len(validFeats))

	for featureIndex := range validFeats {
		op.coefficients[featureIndex] = solved.At(featureIndex+1, 0)
	}

	mapping := make([]string, 0, (columnCount+1)*2)
	mapping = append(mapping, "level", "level")

	for index := 0; index < columnCount; index++ {
		key := "actual_" + string(rune('0'+index))
		mapping = append(mapping, key, key)
	}

	op.input = data.NewMap(mapping...)
	return op
}

func (op *Counterfactual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			level, levelOK := values.Values["level"]

			if !levelOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if len(op.coefficients) > 0 {
				columnCount := len(op.rows[0])
				actualRow := make([]float64, columnCount)

				for index := 0; index < columnCount; index++ {
					key := "actual_" + string(rune('0'+index))
					val, ok := values.Values[key]

					if !ok {
						op.Error(core.ErrNotHeld)
						return
					}

					actualRow[index] = val
				}

				factual := op.intercept

				for featureIndex, column := range op.features {
					factual += op.coefficients[featureIndex] * actualRow[column]
				}

				noise := actualRow[op.target] - factual
				intervened := op.intercept

				for featureIndex, column := range op.features {
					value := actualRow[column]

					if column == op.treatment {
						value = level
					}

					intervened += op.coefficients[featureIndex] * value
				}

				absNoise := math.Abs(noise)
				precision := 1.0 / (1.0 + absNoise)

				op.output.Values["noise"] = noise
				op.output.Values["counterfactual"] = intervened + noise
				op.output.Values["precision"] = precision
				op.output.Values["defined"] = 1.0
			}

			if len(op.coefficients) == 0 {
				actual, actualOK := values.Values["actual"]
				factual, factualOK := values.Values["factual"]
				predicted, predictedOK := values.Values["predicted"]

				if !actualOK || !factualOK || !predictedOK {
					op.Error(core.ErrNotHeld)
					return
				}

				noise := actual - factual
				absNoise := math.Abs(noise)
				precision := 1.0 / (1.0 + absNoise)

				op.output.Values["noise"] = noise
				op.output.Values["counterfactual"] = predicted + noise
				op.output.Values["precision"] = precision
				op.output.Values["defined"] = 1.0
			}

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

package causal

import (
	"fmt"
	"iter"
	"math"
	"slices"
	"unsafe"

	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type stateLayout struct {
	input  data.Map[string]
	output data.Map[float64]
}

type adapterLayout struct {
	primitiveError *core.PrimitiveError
	measurement    *data.Measurement
	state          *stateLayout
	values         data.Map[float64]
}

/*
Table coordinates causal estimation over observational tables.
*/
type Table struct {
	*core.PrimitiveError
	minimum      int
	rows         [][]float64
	target       int
	treatment    int
	features     []int
	linear       bool
	intercept    float64
	coefficients []float64
	baseline     float64
	effect       float64
	stump        *Stump
	backdoor     *Backdoor
	abductive    *Counterfactual
	input        data.Map[string]
	output       data.Map[float64]
}

/*
NewTable fits structural causal models over observational evidence.
*/
func NewTable(
	minimum int,
	rows [][]float64,
	target int,
	treatment int,
	features []int,
	linear bool,
) *Table {
	output := data.NewOutputMap()
	output.Values["expectation"] = 0
	output.Values["counterfactual"] = 0
	output.Values["noise"] = 0
	output.Values["precision"] = 0
	output.Values["defined"] = 0

	prim := &Table{
		PrimitiveError: core.NewPrimitiveError(),
		minimum:        minimum,
		target:         target,
		treatment:      treatment,
		linear:         linear,
		input: data.NewMap(
			"level", "level",
		),
		output: output,
	}

	if minimum < 1 {
		prim.Error(core.ErrDomain)
		return prim
	}

	if len(rows) < minimum {
		prim.Error(core.ErrDomain)
		return prim
	}

	columnCount := len(rows[0])

	if target < 0 || target >= columnCount {
		prim.Error(core.ErrShape)
		return prim
	}

	if treatment < 0 || treatment >= columnCount {
		prim.Error(core.ErrShape)
		return prim
	}

	for _, row := range rows {
		if len(row) != columnCount {
			prim.Error(core.ErrShape)
			return prim
		}
	}

	featureCols := make([]int, 0, len(features)+1)
	seen := make(map[int]bool, len(features)+1)

	for _, column := range append(slices.Clone(features), treatment) {
		if column < 0 || column >= columnCount {
			prim.Error(core.ErrShape)
			return prim
		}

		if column == target || seen[column] {
			continue
		}

		seen[column] = true
		featureCols = append(featureCols, column)
	}

	if len(featureCols) == 0 {
		prim.Error(core.ErrDomain)
		return prim
	}

	observations := make([][]float64, len(rows))

	for rowIndex, row := range rows {
		observations[rowIndex] = slices.Clone(row)
	}

	prim.rows = observations
	prim.features = featureCols

	if !linear {
		prim.stump = NewStump(observations, target, treatment, features)

		if prim.stump.Error() != nil {
			prim.Error(prim.stump.Error())
		}

		return prim
	}

	design := mat.NewDense(len(observations), len(featureCols)+1, nil)
	outcome := mat.NewDense(len(observations), 1, nil)

	for rowIndex, row := range observations {
		design.Set(rowIndex, 0, 1.0)

		for featureIndex, column := range featureCols {
			design.Set(rowIndex, featureIndex+1, row[column])
		}

		outcome.Set(rowIndex, 0, row[target])
	}

	var coefficients mat.Dense
	err := coefficients.Solve(design, outcome)

	if err != nil {
		prim.Error(core.ErrDomain)
		return prim
	}

	intercept := coefficients.At(0, 0)
	coeffs := make([]float64, len(featureCols))
	treatmentEffect := 0.0
	covariateSum := 0.0

	for featureIndex, column := range featureCols {
		coeff := coefficients.At(featureIndex+1, 0)
		coeffs[featureIndex] = coeff

		if column == treatment {
			treatmentEffect = coeff
		}

		if column != treatment {
			colSum := 0.0

			for _, row := range observations {
				colSum += row[column]
			}

			meanCol := colSum / float64(len(observations))
			covariateSum += coeff * meanCol
		}
	}

	prim.intercept = intercept
	prim.coefficients = coeffs
	prim.baseline = intercept + covariateSum
	prim.effect = treatmentEffect
	prim.backdoor = NewBackdoor(1e-15)
	prim.abductive = NewCounterfactual(1e-15)

	return prim
}

/*
Next evaluates interventional expectation or counterfactual queries.
*/
func (op *Table) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if !op.linear {
				once := func(forward func(unsafe.Pointer) bool) {
					forward(arriving)
				}

				for out := range op.stump.Next(once) {
					if !yield(out) {
						return
					}
				}

				if err := op.stump.Error(); err != nil {
					op.Error(err)
					return
				}

				continue
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

			inspector := (*adapterLayout)(unsafe.Pointer(adapter))
			hasActual := false

			if inspector != nil && inspector.state != nil {
				_, hasActual = inspector.state.output.Values["actual_0"]
			}

			if hasActual {
				columnCount := len(op.rows[0])
				actualRow := make([]float64, columnCount)

				for colIndex := 0; colIndex < columnCount; colIndex++ {
					key := fmt.Sprintf("actual_%d", colIndex)
					actualVal, ok := inspector.state.output.Values[key]

					if !ok {
						op.Error(core.ErrShape)
						return
					}

					actualRow[colIndex] = actualVal
				}

				factualPrediction := op.intercept
				interventionalPrediction := op.intercept

				for featureIndex, column := range op.features {
					factualVal := actualRow[column]
					interventionalVal := actualRow[column]

					if column == op.treatment {
						interventionalVal = level
					}

					factualPrediction += op.coefficients[featureIndex] * factualVal
					interventionalPrediction += op.coefficients[featureIndex] * interventionalVal
				}

				noise := actualRow[op.target] - factualPrediction
				counterfactual := interventionalPrediction + noise
				precision := 1.0 / (1.0 + math.Abs(noise))

				op.output.Values["counterfactual"] = counterfactual
				op.output.Values["noise"] = noise
				op.output.Values["precision"] = precision
				op.output.Values["defined"] = 1.0

				for range adapter.Next(data.NewValue(op.output)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				if !yield(arriving) {
					return
				}

				continue
			}

			expectation := op.baseline + op.effect*level
			op.output.Values["expectation"] = expectation
			op.output.Values["defined"] = 1.0

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

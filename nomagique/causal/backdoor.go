package causal

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"unsafe"

	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Backdoor standardizes predictions over observed evidence after replacing the
treatment coordinate with the interventional level.
*/
type Backdoor struct {
	*core.PrimitiveError
	tolerance    float64
	intercept    float64
	coefficients []float64
	treatment    int
	features     []int
	rows         [][]float64
	input        data.Map[string]
	output       data.Map[float64]
}

func NewBackdoor(tolerance float64) *Backdoor {
	output := data.NewOutputMap()
	output.Values["expectation"] = 0
	output.Values["defined"] = 0

	return &Backdoor{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
		input: data.NewMap(
			"level", "level",
			"baseline", "baseline",
			"effect", "effect",
		),
		output: output,
	}
}

func (op *Backdoor) SetEvidence(
	rows [][]float64,
	target int,
	treatment int,
	controls []int,
) error {
	copied, err := op.copyRows(rows, target, 1)

	if err != nil {
		return op.Error(err)
	}

	features, err := op.validateFeatures(len(copied[0]), target, treatment, controls)

	if err != nil {
		return op.Error(err)
	}

	err = op.fit(copied, target, features)

	if err != nil {
		return op.Error(err)
	}

	op.rows = copied
	op.treatment = treatment
	op.features = features
	op.input = data.NewMap("level", "level")
	return nil
}

func (op *Backdoor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
				expectation := 0.0

				for _, observation := range op.rows {
					expectation += op.predictWithIntervention(observation, op.treatment, level)
				}

				op.output.Values["expectation"] = expectation / float64(len(op.rows))
				op.output.Values["defined"] = 1.0
			}

			if len(op.coefficients) == 0 {
				baseline, baselineOK := values.Values["baseline"]
				effect, effectOK := values.Values["effect"]

				if !baselineOK || !effectOK {
					op.Error(core.ErrNotHeld)
					return
				}

				op.output.Values["expectation"] = baseline + effect*level
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

func (op *Backdoor) fit(rows [][]float64, target int, features []int) error {
	design := mat.NewDense(len(rows), len(features)+1, nil)
	outcome := mat.NewDense(len(rows), 1, nil)

	for rowIndex, row := range rows {
		design.Set(rowIndex, 0, 1)

		for featureIndex, column := range features {
			design.Set(rowIndex, featureIndex+1, row[column])
		}

		outcome.Set(rowIndex, 0, row[target])
	}

	var solved mat.Dense
	err := solved.Solve(design, outcome)

	if err != nil {
		return io.EOF
	}

	op.intercept = solved.At(0, 0)
	op.coefficients = make([]float64, len(features))

	for featureIndex := range features {
		op.coefficients[featureIndex] = solved.At(featureIndex+1, 0)
	}

	return nil
}

func (op *Backdoor) predictWithIntervention(row []float64, treatment int, level float64) float64 {
	prediction := op.intercept

	for featureIndex, column := range op.features {
		value := row[column]

		if column == treatment {
			value = level
		}

		prediction += op.coefficients[featureIndex] * value
	}

	return prediction
}

func (op *Backdoor) copyRows(rows [][]float64, target int, minimum int) ([][]float64, error) {
	if target < 0 {
		return nil, fmt.Errorf(
			"causal: target column must be non-negative: %w", core.ErrDomain,
		)
	}

	if minimum < 1 {
		return nil, fmt.Errorf(
			"causal: minimum rows must be positive: %w", core.ErrDomain,
		)
	}

	if len(rows) < minimum {
		return nil, fmt.Errorf(
			"causal: %d observational rows available; need %d: %w",
			len(rows), minimum, core.ErrDomain,
		)
	}

	columnCount := len(rows[0])

	if columnCount == 0 || target >= columnCount {
		return nil, fmt.Errorf(
			"causal: target column %d is outside row width %d: %w",
			target, columnCount, core.ErrShape,
		)
	}

	observations := make([][]float64, len(rows))

	for rowIndex, row := range rows {
		if len(row) != columnCount {
			return nil, fmt.Errorf(
				"causal: row %d has width %d; expected %d: %w",
				rowIndex, len(row), columnCount, core.ErrShape,
			)
		}

		observations[rowIndex] = slices.Clone(row)
	}

	return observations, nil
}

func (op *Backdoor) validateFeatures(
	columnCount int,
	target int,
	treatment int,
	requested []int,
) ([]int, error) {
	if treatment < 0 || treatment >= columnCount {
		return nil, fmt.Errorf(
			"causal: treatment column %d is outside row width %d: %w",
			treatment, columnCount, core.ErrShape,
		)
	}

	features := make([]int, 0, len(requested)+1)
	seen := make(map[int]bool, len(requested)+1)

	for _, feature := range append(slices.Clone(requested), treatment) {
		if feature < 0 || feature >= columnCount {
			return nil, fmt.Errorf(
				"causal: feature column %d is outside row width %d: %w",
				feature, columnCount, core.ErrShape,
			)
		}

		if feature == target || seen[feature] {
			continue
		}

		seen[feature] = true
		features = append(features, feature)
	}

	if len(features) == 0 {
		return nil, fmt.Errorf(
			"causal: no explanatory features remain after validation: %w",
			core.ErrDomain,
		)
	}

	return features, nil
}

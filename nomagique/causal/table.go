package causal

import (
	"fmt"
	"io"
	"math"
	"slices"

	"gonum.org/v1/gonum/mat"
)

/*
Table owns observational rows used for interventional and counterfactual fits.
Rows are copied at the boundary so search simulations cannot mutate evidence.
*/
type Table struct {
	rows    [][]float64
	target  int
	minimum int
	linear  bool
}

/*
NewTable validates and copies observational rows for one target model.
*/
func NewTable(
	rows [][]float64,
	target int,
	minimum int,
	linear bool,
) (*Table, error) {
	if target < 0 {
		return nil, fmt.Errorf("causal: target column must be non-negative")
	}

	if minimum < 1 {
		return nil, fmt.Errorf("causal: minimum rows must be positive")
	}

	if len(rows) < minimum {
		return nil, fmt.Errorf(
			"causal: %d observational rows available; need %d",
			len(rows), minimum,
		)
	}

	columnCount := len(rows[0])

	if columnCount == 0 || target >= columnCount {
		return nil, fmt.Errorf("causal: target column %d is outside row width %d", target, columnCount)
	}

	observations := make([][]float64, len(rows))

	for rowIndex, row := range rows {
		if len(row) != columnCount {
			return nil, fmt.Errorf(
				"causal: row %d has width %d; expected %d",
				rowIndex, len(row), columnCount,
			)
		}

		observations[rowIndex] = slices.Clone(row)
	}

	return &Table{
		rows:    observations,
		target:  target,
		minimum: minimum,
		linear:  linear,
	}, nil
}

/*
Rows returns an isolated copy of the observational evidence.
*/
func (table *Table) Rows() [][]float64 {
	rows := make([][]float64, len(table.rows))

	for rowIndex, row := range table.rows {
		rows[rowIndex] = slices.Clone(row)
	}

	return rows
}

/*
DoExpectation estimates E[target | do(treatment=level)] through empirical
backdoor standardization over the observed control distribution.
*/
func (table *Table) DoExpectation(
	treatment int,
	level float64,
	controls ...int,
) (float64, error) {
	features, err := validatedFeatures(
		len(table.rows[0]), table.target, treatment, controls,
	)

	if err != nil {
		return 0, err
	}

	predictor, err := fitPredictor(table.rows, table.target, features, table.linear)

	if err != nil {
		return 0, err
	}

	expectation := 0.0

	for _, observation := range table.rows {
		intervention := slices.Clone(observation)
		intervention[treatment] = level
		expectation += predictor.Predict(intervention)
	}

	return expectation / float64(len(table.rows)), nil
}

/*
AbductiveCounterfactual performs abduction, intervention, then prediction. The
returned precision is a bounded audit weight derived from reconstruction error.
*/
func (table *Table) AbductiveCounterfactual(
	features []int,
	actual []float64,
	treatment int,
	level float64,
) (counterfactual float64, noise float64, precision float64, err error) {
	if len(actual) != len(table.rows[0]) {
		return 0, 0, 0, fmt.Errorf(
			"causal: actual row has width %d; expected %d",
			len(actual), len(table.rows[0]),
		)
	}

	validated, err := validatedFeatures(
		len(actual), table.target, treatment, features,
	)

	if err != nil {
		return 0, 0, 0, err
	}

	predictor, err := fitPredictor(table.rows, table.target, validated, table.linear)

	if err != nil {
		return 0, 0, 0, err
	}

	factualPrediction := predictor.Predict(actual)
	noise = actual[table.target] - factualPrediction
	intervention := slices.Clone(actual)
	intervention[treatment] = level
	counterfactual = predictor.Predict(intervention) + noise
	precision = 1 / (1 + math.Abs(noise))

	return counterfactual, noise, precision, nil
}

/*
DoExpectation preserves the old nomagique engine boundary while using Table.
*/
func DoExpectation(
	rows [][]float64,
	target int,
	minimum int,
	treatment int,
	level float64,
	controls []int,
) (float64, error) {
	table, err := NewTable(rows, target, minimum, true)

	if err != nil {
		return 0, err
	}

	return table.DoExpectation(treatment, level, controls...)
}

/*
AbductiveCounterfactual preserves the old nomagique engine boundary.
*/
func AbductiveCounterfactual(
	rows [][]float64,
	target int,
	minimum int,
	features []int,
	linear bool,
	actual []float64,
	treatment int,
	level float64,
) (counterfactual float64, noise float64, err error) {
	table, err := NewTable(rows, target, minimum, linear)

	if err != nil {
		return 0, 0, err
	}

	counterfactual, noise, _, err = table.AbductiveCounterfactual(
		features, actual, treatment, level,
	)
	return counterfactual, noise, err
}

func validatedFeatures(
	columnCount int,
	target int,
	treatment int,
	requested []int,
) ([]int, error) {
	if treatment < 0 || treatment >= columnCount {
		return nil, fmt.Errorf(
			"causal: treatment column %d is outside row width %d",
			treatment, columnCount,
		)
	}

	features := make([]int, 0, len(requested)+1)
	seen := make(map[int]bool, len(requested)+1)

	for _, feature := range append(slices.Clone(requested), treatment) {
		if feature < 0 || feature >= columnCount {
			return nil, fmt.Errorf(
				"causal: feature column %d is outside row width %d",
				feature, columnCount,
			)
		}

		if feature == target || seen[feature] {
			continue
		}

		seen[feature] = true
		features = append(features, feature)
	}

	if len(features) == 0 {
		return nil, fmt.Errorf("causal: no explanatory features remain after validation")
	}

	return features, nil
}

type predictor interface {
	Predict([]float64) float64
}

type linearPredictor struct {
	intercept    float64
	coefficients []float64
	features     []int
}

func (predictor *linearPredictor) Predict(row []float64) float64 {
	prediction := predictor.intercept

	for featureIndex, column := range predictor.features {
		prediction += predictor.coefficients[featureIndex] * row[column]
	}

	return prediction
}

type stump struct {
	column    int
	threshold float64
	below     float64
	above     float64
}

type stumpPredictor struct {
	baseline float64
	stumps   []stump
}

func (predictor *stumpPredictor) Predict(row []float64) float64 {
	prediction := predictor.baseline

	for _, decision := range predictor.stumps {
		if row[decision.column] <= decision.threshold {
			prediction += decision.below
			continue
		}

		prediction += decision.above
	}

	return prediction
}

func fitPredictor(
	rows [][]float64,
	target int,
	features []int,
	linear bool,
) (predictor, error) {
	if linear {
		return fitLinear(rows, target, features)
	}

	return fitStumps(rows, target, features), nil
}

func fitLinear(
	rows [][]float64,
	target int,
	features []int,
) (predictor, error) {
	design := mat.NewDense(len(rows), len(features)+1, nil)
	outcome := mat.NewDense(len(rows), 1, nil)

	for rowIndex, row := range rows {
		design.Set(rowIndex, 0, 1)

		for featureIndex, column := range features {
			design.Set(rowIndex, featureIndex+1, row[column])
		}

		outcome.Set(rowIndex, 0, row[target])
	}

	var coefficients mat.Dense
	err := coefficients.Solve(design, outcome)

	if err != nil {
		// A singular or rank-deficient design (a constant treatment or
		// control column, exact collinearity, or fewer distinct rows than
		// parameters) is a genuinely non-identifiable structural model, not
		// a fatal failure. Report io.EOF — the same non-identifiable signal
		// the association, effect-scale, and residualization paths use — so
		// the causal ladder resolves to an explicit unresolved state instead
		// of crashing the pipeline.
		return nil, io.EOF
	}

	model := &linearPredictor{
		intercept:    coefficients.At(0, 0),
		coefficients: make([]float64, len(features)),
		features:     slices.Clone(features),
	}

	for featureIndex := range features {
		model.coefficients[featureIndex] = coefficients.At(featureIndex+1, 0)
	}

	return model, nil
}

func fitStumps(
	rows [][]float64,
	target int,
	features []int,
) predictor {
	baseline := meanColumn(rows, target)
	model := &stumpPredictor{baseline: baseline}
	residuals := make([]float64, len(rows))

	for rowIndex, row := range rows {
		residuals[rowIndex] = row[target] - baseline
	}

	for _, column := range features {
		threshold := meanColumn(rows, column)
		belowSum := 0.0
		belowCount := 0
		aboveSum := 0.0
		aboveCount := 0

		for rowIndex, row := range rows {
			if row[column] <= threshold {
				belowSum += residuals[rowIndex]
				belowCount++
				continue
			}

			aboveSum += residuals[rowIndex]
			aboveCount++
		}

		decision := stump{column: column, threshold: threshold}

		if belowCount > 0 {
			decision.below = belowSum / float64(belowCount)
		}

		if aboveCount > 0 {
			decision.above = aboveSum / float64(aboveCount)
		}

		model.stumps = append(model.stumps, decision)
	}

	return model
}

func meanColumn(rows [][]float64, column int) float64 {
	total := 0.0

	for _, row := range rows {
		total += row[column]
	}

	return total / float64(len(rows))
}

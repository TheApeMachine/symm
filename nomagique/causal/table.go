package causal

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"slices"
	"unsafe"

	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
TableInput commands one observational-table evaluation. A non-nil Actual
requests an abductive counterfactual over Features; a nil Actual requests the
backdoor interventional expectation of Treatment at Level over Controls. Linear
selects the affine strategy; otherwise stumps fit the structural model.
*/
type TableInput struct {
	Rows      [][]float64
	Target    int
	Treatment int
	Level     float64
	Controls  []int
	Features  []int
	Actual    []float64
	Linear    bool
}

/*
TableOutput carries the commanded result: the interventional expectation, or
the abductive counterfactual with its retained noise and bounded audit
precision derived from reconstruction error.
*/
type TableOutput struct {
	Expectation    float64
	Counterfactual float64
	Noise          float64
	Precision      float64
}

/*
Table owns observational rows used for interventional and counterfactual fits.
Rows are copied at the boundary so search simulations cannot mutate evidence.
The fitting strategies (affine least squares, stumps) are internal.
*/
type Table struct {
	err     error
	minimum int
	out     TableOutput
}

/*
NewTable creates a table primitive requiring at least minimum observational
rows per command.
*/
func NewTable(minimum int) core.Primitive {
	return &Table{minimum: minimum}
}

/*
Next evaluates each arriving command and yields its result payload.
*/
func (op *Table) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*TableInput)(arriving)

			rows, err := copyRows(command.Rows, command.Target, op.minimum)
			if err != nil {
				op.Error(err)
				return
			}

			op.out = TableOutput{}

			if command.Actual != nil {
				err = op.abduct(rows, command)
			} else {
				err = op.standardize(rows, command)
			}

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Table) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
standardize estimates E[target | do(treatment=level)] through empirical
backdoor standardization over the observed control distribution.
*/
func (op *Table) standardize(rows [][]float64, command *TableInput) error {
	features, err := validatedFeatures(
		len(rows[0]), command.Target, command.Treatment, command.Controls,
	)
	if err != nil {
		return err
	}

	model, err := fitPredictor(rows, command.Target, features, command.Linear)
	if err != nil {
		return err
	}

	expectation := 0.0

	for _, observation := range rows {
		expectation += model.predictWithIntervention(
			observation, command.Treatment, command.Level,
		)
	}

	op.out.Expectation = expectation / float64(len(rows))
	return nil
}

/*
abduct performs abduction, intervention, then prediction, retaining the
factual noise in the counterfactual.
*/
func (op *Table) abduct(rows [][]float64, command *TableInput) error {
	if len(command.Actual) != len(rows[0]) {
		return fmt.Errorf(
			"causal: actual row has width %d; expected %d: %w",
			len(command.Actual), len(rows[0]), core.ErrShape,
		)
	}

	features, err := validatedFeatures(
		len(command.Actual), command.Target, command.Treatment, command.Features,
	)
	if err != nil {
		return err
	}

	model, err := fitPredictor(rows, command.Target, features, command.Linear)
	if err != nil {
		return err
	}

	factual := model.predict(command.Actual)
	op.out.Noise = command.Actual[command.Target] - factual
	op.out.Counterfactual = model.predictWithIntervention(
		command.Actual, command.Treatment, command.Level,
	) + op.out.Noise
	op.out.Precision = 1 / (1 + math.Abs(op.out.Noise))
	return nil
}

/*
copyRows validates one command's evidence and copies it so later search
simulations cannot mutate it.
*/
func copyRows(rows [][]float64, target int, minimum int) ([][]float64, error) {
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

/*
validatedFeatures deduplicates the requested explanatory columns with the
treatment and removes the outcome, refusing an empty remainder.
*/
func validatedFeatures(
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

type predictor interface {
	predict([]float64) float64
	predictWithIntervention([]float64, int, float64) float64
}

type linearPredictor struct {
	intercept    float64
	coefficients []float64
	features     []int
}

func (predictor *linearPredictor) predict(row []float64) float64 {
	prediction := predictor.intercept

	for featureIndex, column := range predictor.features {
		prediction += predictor.coefficients[featureIndex] * row[column]
	}

	return prediction
}

func (predictor *linearPredictor) predictWithIntervention(row []float64, treatment int, level float64) float64 {
	prediction := predictor.intercept

	for featureIndex, column := range predictor.features {
		value := row[column]

		if column == treatment {
			value = level
		}

		prediction += predictor.coefficients[featureIndex] * value
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

func (predictor *stumpPredictor) predict(row []float64) float64 {
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

func (predictor *stumpPredictor) predictWithIntervention(row []float64, treatment int, level float64) float64 {
	prediction := predictor.baseline

	for _, decision := range predictor.stumps {
		value := row[decision.column]

		if decision.column == treatment {
			value = level
		}

		if value <= decision.threshold {
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

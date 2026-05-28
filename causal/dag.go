package causal

import (
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/symm/numeric"
)

type dagNodeTable struct {
	rows   [][]float64
	target int
}

type dagLinearModel struct {
	coefficients []float64
	predictors   []int
}

func newDAGNodeTable(
	rows [][]float64,
	target int,
	minRows int,
) (dagNodeTable, error) {
	if minRows <= 0 {
		return dagNodeTable{}, errors.New("causal: DAG minRows must be positive")
	}

	if len(rows) < minRows {
		return dagNodeTable{}, fmt.Errorf("causal: DAG needs %d rows, got %d", minRows, len(rows))
	}

	nodeCount := len(rows[0])

	if nodeCount == 0 {
		return dagNodeTable{}, errors.New("causal: DAG rows must contain nodes")
	}

	if target < 0 || target >= nodeCount {
		return dagNodeTable{}, fmt.Errorf("causal: target node %d outside row width %d", target, nodeCount)
	}

	for rowIndex, row := range rows {
		if len(row) != nodeCount {
			return dagNodeTable{}, fmt.Errorf(
				"causal: row %d width %d differs from %d",
				rowIndex,
				len(row),
				nodeCount,
			)
		}
	}

	return dagNodeTable{
		rows:   rows,
		target: target,
	}, nil
}

func (nodeTable dagNodeTable) Association(treatment int) (float64, error) {
	treatmentValues, err := nodeTable.column(treatment)

	if err != nil {
		return 0, err
	}

	targetValues, err := nodeTable.column(nodeTable.target)

	if err != nil {
		return 0, err
	}

	return pearson(treatmentValues, targetValues), nil
}

func (nodeTable dagNodeTable) BackdoorEffect(
	treatment int,
	controls ...int,
) (float64, error) {
	treatmentValues, err := nodeTable.column(treatment)

	if err != nil {
		return 0, err
	}

	targetValues, err := nodeTable.column(nodeTable.target)

	if err != nil {
		return 0, err
	}

	controlColumns, err := nodeTable.columns(controls...)

	if err != nil {
		return 0, err
	}

	residualTarget, ok := residualize(targetValues, controlColumns...)

	if !ok {
		return 0, errors.New("causal: target residualization failed")
	}

	residualTreatment, ok := residualize(treatmentValues, controlColumns...)

	if !ok {
		return 0, errors.New("causal: treatment residualization failed")
	}

	denominator := math.Max(dot(residualTreatment, residualTreatment), minBackdoorDenominator)

	return dot(residualTarget, residualTreatment) / denominator, nil
}

func (nodeTable dagNodeTable) LinearModel(
	predictors ...int,
) (dagLinearModel, error) {
	targetValues, err := nodeTable.column(nodeTable.target)

	if err != nil {
		return dagLinearModel{}, err
	}

	predictorColumns, err := nodeTable.columns(predictors...)

	if err != nil {
		return dagLinearModel{}, err
	}

	coefficients, ok := ols(targetValues, predictorColumns...)

	if !ok {
		return dagLinearModel{}, errors.New("causal: linear structural fit failed")
	}

	return dagLinearModel{
		coefficients: coefficients,
		predictors:   append([]int(nil), predictors...),
	}, nil
}

func (nodeTable dagNodeTable) Percentile(node int, percentile float64) (float64, error) {
	values, err := nodeTable.column(node)

	if err != nil {
		return 0, err
	}

	if len(values) == 0 {
		return 0, errors.New("causal: percentile node has no values")
	}

	return numeric.PercentileSorted(numeric.CopySorted(values), percentile), nil
}

func (nodeTable dagNodeTable) KernelBackdoorEffect(
	treatment int,
	bandwidth float64,
	controls ...int,
) (float64, error) {
	if bandwidth <= 0 {
		return 0, errors.New("causal: kernel bandwidth must be positive")
	}

	if err := nodeTable.validateNode(treatment); err != nil {
		return 0, err
	}

	if err := nodeTable.validateNodes(controls...); err != nil {
		return 0, err
	}

	features := append([]int(nil), controls...)
	features = append(features, treatment)
	current := nodeTable.rows[len(nodeTable.rows)-1]
	numerator := 0.0
	denominator := 0.0

	for _, row := range nodeTable.rows {
		distance := nodeDistance(current, row, features)
		weight := math.Exp(-distance * distance / (2 * bandwidth * bandwidth))

		if weight < minKernelWeight {
			continue
		}

		treatmentValue := row[treatment]
		numerator += weight * row[nodeTable.target] * treatmentValue
		denominator += weight * treatmentValue * treatmentValue
	}

	denominator = math.Max(denominator, minBackdoorDenominator)

	return numerator / denominator, nil
}

func (model dagLinearModel) Predict(
	row []float64,
	overrideNode int,
	overrideValue float64,
) (float64, error) {
	if len(model.coefficients) != len(model.predictors)+1 {
		return 0, errors.New("causal: linear model coefficient shape is invalid")
	}

	if len(row) == 0 {
		return 0, errors.New("causal: prediction row is empty")
	}

	prediction := model.coefficients[0]

	for predictorIndex, node := range model.predictors {
		if node < 0 || node >= len(row) {
			return 0, fmt.Errorf("causal: predictor node %d outside row width %d", node, len(row))
		}

		value := row[node]

		if node == overrideNode {
			value = overrideValue
		}

		prediction += model.coefficients[predictorIndex+1] * value
	}

	return prediction, nil
}

func (model dagLinearModel) CounterfactualUplift(
	row []float64,
	treatment int,
	intervention float64,
) (float64, error) {
	observed, err := model.Predict(row, -1, 0)

	if err != nil {
		return 0, err
	}

	counterfactual, err := model.Predict(row, treatment, intervention)

	if err != nil {
		return 0, err
	}

	return counterfactual - observed, nil
}

func (nodeTable dagNodeTable) column(node int) ([]float64, error) {
	if err := nodeTable.validateNode(node); err != nil {
		return nil, err
	}

	values := make([]float64, len(nodeTable.rows))

	for rowIndex, row := range nodeTable.rows {
		values[rowIndex] = row[node]
	}

	return values, nil
}

func (nodeTable dagNodeTable) columns(nodes ...int) ([][]float64, error) {
	if err := nodeTable.validateNodes(nodes...); err != nil {
		return nil, err
	}

	columns := make([][]float64, 0, len(nodes))

	for _, node := range nodes {
		column, err := nodeTable.column(node)

		if err != nil {
			return nil, err
		}

		columns = append(columns, column)
	}

	return columns, nil
}

func (nodeTable dagNodeTable) validateNodes(nodes ...int) error {
	for _, node := range nodes {
		if err := nodeTable.validateNode(node); err != nil {
			return err
		}
	}

	return nil
}

func (nodeTable dagNodeTable) validateNode(node int) error {
	if len(nodeTable.rows) == 0 {
		return errors.New("causal: DAG table is empty")
	}

	width := len(nodeTable.rows[0])

	if node < 0 || node >= width {
		return fmt.Errorf("causal: node %d outside row width %d", node, width)
	}

	return nil
}

func nodeDistance(left, right []float64, features []int) float64 {
	sum := 0.0

	for _, feature := range features {
		delta := left[feature] - right[feature]
		sum += delta * delta
	}

	return math.Sqrt(sum)
}

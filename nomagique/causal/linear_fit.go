package causal

import (
	"io"
	"math"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

func (nodeTable nodeTable) treatmentIdentifiable(
	treatment int,
	controls ...int,
) error {
	treatmentColumn, err := nodeTable.column(treatment)

	if err != nil {
		return err
	}

	treatmentMean := stat.Mean(treatmentColumn, nil)
	treatmentScale := stat.StdDev(treatmentColumn, nil)

	if treatmentScale <= 0 {
		return io.EOF
	}

	controlColumns, err := nodeTable.columns(controls...)

	if err != nil {
		return err
	}

	variableControls := make([][]float64, 0, len(controlColumns))

	for _, column := range controlColumns {
		if stat.StdDev(column, nil) > 0 {
			variableControls = append(variableControls, column)
		}
	}

	controlRank, err := standardizedRank(variableControls...)

	if err != nil {
		return err
	}

	standardizedTreatment := make([]float64, len(treatmentColumn))

	for index, value := range treatmentColumn {
		standardizedTreatment[index] = (value - treatmentMean) / treatmentScale
	}

	combined := append(variableControls, standardizedTreatment)
	combinedRank, err := standardizedRank(combined...)

	if err != nil {
		return err
	}

	if combinedRank <= controlRank {
		return io.EOF
	}

	return nil
}

func standardizedRank(columns ...[]float64) (int, error) {
	if len(columns) == 0 {
		return 0, nil
	}

	rowCount := len(columns[0])
	design := mat.NewDense(rowCount, len(columns), nil)

	for columnIndex, column := range columns {
		if len(column) != rowCount {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"causal: rank column length mismatch",
				nil,
			))
		}

		mean := stat.Mean(column, nil)
		scale := stat.StdDev(column, nil)

		for rowIndex, value := range column {
			standardized := value

			if scale > 0 {
				standardized = (value - mean) / scale
			}

			design.Set(rowIndex, columnIndex, standardized)
		}
	}

	var decomposition mat.SVD

	if !decomposition.Factorize(design, mat.SVDThin) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: rank decomposition failed",
			nil,
		))
	}

	singularValues := decomposition.Values(nil)
	machineEpsilon := math.Nextafter(1, 2) - 1
	tolerance := machineEpsilon * float64(max(rowCount, len(columns))) * singularValues[0]

	return decomposition.Rank(tolerance), nil
}

func backdoorDenominator(residualNorm float64) (float64, error) {
	if residualNorm <= 0 {
		return 0, io.EOF
	}

	if math.IsNaN(residualNorm) || math.IsInf(residualNorm, 0) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: backdoor denominator is non-positive",
			nil,
		))
	}

	return residualNorm, nil
}

func olsFit(target []float64, predictors ...[]float64) ([]float64, error) {
	if len(target) < 2 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: ols requires at least two target samples",
			nil,
		))
	}

	for _, value := range target {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"causal: ols target is non-finite",
				nil,
			))
		}
	}

	for _, predictor := range predictors {
		if len(predictor) != len(target) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"causal: ols predictor length mismatch",
				nil,
			))
		}

		for _, value := range predictor {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"causal: ols predictor is non-finite",
					nil,
				))
			}
		}
	}

	rowCount := len(target)
	columnCount := len(predictors) + 1

	if rowCount < columnCount {
		return nil, io.EOF
	}

	targetMean := stat.Mean(target, nil)
	targetScale := stat.StdDev(target, nil)

	if targetScale <= 0 {
		return nil, io.EOF
	}

	predictorMeans := make([]float64, len(predictors))
	predictorScales := make([]float64, len(predictors))
	design := mat.NewDense(rowCount, columnCount, nil)
	standardizedTarget := make([]float64, rowCount)

	for rowIndex, value := range target {
		design.Set(rowIndex, 0, 1)
		standardizedTarget[rowIndex] = (value - targetMean) / targetScale
	}

	for predictorIndex, predictor := range predictors {
		predictorMeans[predictorIndex] = stat.Mean(predictor, nil)
		predictorScales[predictorIndex] = stat.StdDev(predictor, nil)

		for rowIndex, value := range predictor {
			standardized := 0.0

			if predictorScales[predictorIndex] > 0 {
				standardized = (value - predictorMeans[predictorIndex]) /
					predictorScales[predictorIndex]
			}

			design.Set(
				rowIndex,
				predictorIndex+1,
				standardized,
			)
		}
	}

	var decomposition mat.SVD

	if !decomposition.Factorize(design, mat.SVDThin) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: ols decomposition failed",
			nil,
		))
	}

	singularValues := decomposition.Values(nil)
	machineEpsilon := math.Nextafter(1, 2) - 1
	rankTolerance := machineEpsilon * float64(max(rowCount, columnCount)) * singularValues[0]
	rank := decomposition.Rank(rankTolerance)

	if rank < 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: ols intercept is not identifiable",
			nil,
		))
	}

	standardizedSolution := mat.NewVecDense(columnCount, nil)
	decomposition.SolveVecTo(
		standardizedSolution,
		mat.NewVecDense(rowCount, standardizedTarget),
		rank,
	)

	coefficients := make([]float64, columnCount)
	coefficients[0] = targetMean + targetScale*standardizedSolution.AtVec(0)

	for predictorIndex := range predictors {
		if predictorScales[predictorIndex] <= 0 {
			continue
		}

		coefficient := targetScale * standardizedSolution.AtVec(predictorIndex+1) /
			predictorScales[predictorIndex]
		coefficients[predictorIndex+1] = coefficient
		coefficients[0] -= coefficient * predictorMeans[predictorIndex]
	}

	return coefficients, nil
}

func residualize(target []float64, controls ...[]float64) ([]float64, error) {
	if len(controls) == 0 {
		return append([]float64(nil), target...), nil
	}

	coefficients, err := olsFit(target, controls...)

	if err != nil {
		return nil, err
	}

	residuals := make([]float64, len(target))

	for index := range target {
		fitted := coefficients[0]

		for controlIndex, control := range controls {
			fitted += coefficients[controlIndex+1] * control[index]
		}

		residuals[index] = target[index] - fitted
	}

	return residuals, nil
}

func vectorDot(left, right []float64) (float64, error) {
	if len(left) != len(right) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: dot length mismatch",
			nil,
		))
	}

	return floats.Dot(left, right), nil
}

func pairConditionNumber(left, right []float64) (float64, error) {
	if len(left) != len(right) || len(left) == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: pair condition number requires equal non-empty columns",
			nil,
		))
	}

	correlation := math.Abs(stat.Correlation(left, right, nil))

	if math.IsNaN(correlation) || math.IsInf(correlation, 0) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: pair correlation is non-finite",
			nil,
		))
	}

	if correlation >= 1 {
		return math.Inf(1), nil
	}

	return (1 + correlation) / (1 - correlation), nil
}

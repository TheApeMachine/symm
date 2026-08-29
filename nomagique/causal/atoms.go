package causal

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
	"gonum.org/v1/gonum/stat"
)

/*
Frame slot symbols for the causal ladder atoms. The retained row window is held
in the generic sample/N slots in row-major order; the shape and role symbols
declare how to read it, and the result symbols carry each ladder channel out.

These symbols are structural: they name how the window is laid out and where
each measurement lands, never what the data means.
*/
var (
	SymbolRowCount  = types.MustIntern("causal/row_count")
	SymbolTarget    = types.MustIntern("causal/target")
	SymbolTreatment = types.MustIntern("causal/treatment")
	SymbolLevel     = types.MustIntern("causal/level")
	SymbolBandwidth = types.MustIntern("causal/bandwidth")

	SymbolAssociation    = types.MustIntern("causal/association")
	SymbolBackdoor       = types.MustIntern("causal/backdoor")
	SymbolDoExpectation  = types.MustIntern("causal/do_expectation")
	SymbolCounterfactual = types.MustIntern("causal/counterfactual")
	SymbolNoise          = types.MustIntern("causal/noise")
	SymbolUplift         = types.MustIntern("causal/uplift")
	SymbolTreatmentScale = types.MustIntern("causal/treatment_scale")
	SymbolTargetScale    = types.MustIntern("causal/target_scale")
)

/*
readWindow reads the retained row window out of the generic sample slots. The
row count is declared by SymbolRowCount; the width is the number of populated
sample slots divided by the row count (the window is always a complete, full
rectangle).
*/
func readWindow(input types.Frame) ([][]float64, error) {
	rowCount, hasRowCount := input.Get(SymbolRowCount)

	if !hasRowCount {
		return nil, fmt.Errorf("causal: row count required")
	}

	rows := int(rowCount)

	if rows < 1 {
		return nil, io.EOF
	}

	values := make([]float64, 0, types.MaxSamples)

	for index := range types.MaxSamples {
		value, found := input.Get(types.MustSampleSymbol(index))

		if !found {
			break
		}

		values = append(values, value)
	}

	if len(values) == 0 || len(values)%rows != 0 {
		return nil, fmt.Errorf("causal: window is not a complete rectangle")
	}

	columns := len(values) / rows
	table := make([][]float64, rows)

	for rowIndex := 0; rowIndex < rows; rowIndex++ {
		table[rowIndex] = values[rowIndex*columns : (rowIndex+1)*columns]
	}

	return table, nil
}

func columnValues(table [][]float64, column int) []float64 {
	values := make([]float64, len(table))

	for rowIndex, row := range table {
		if column < 0 || column >= len(row) {
			values[rowIndex] = 0
			continue
		}

		values[rowIndex] = row[column]
	}

	return values
}

/*
Association writes the Pearson correlation between the treatment and target
columns to SymbolAssociation. A column with no dispersion reports io.EOF.
*/
func Association(input *types.Frame) {
	target, hasTarget := input.Get(SymbolTarget)
	treatment, hasTreatment := input.Get(SymbolTreatment)

	if !hasTarget || !hasTreatment {
		input.Err = fmt.Errorf("causal: association requires target and treatment roles")
		return
	}

	table, err := readWindow(*input)

	if err != nil {
		input.Err = err
		return
	}

	treatmentValues := columnValues(table, int(treatment))
	targetValues := columnValues(table, int(target))

	if stat.Variance(treatmentValues, nil) <= 0 ||
		stat.Variance(targetValues, nil) <= 0 {
		input.Err = io.EOF
		return
	}

	association := stat.Correlation(treatmentValues, targetValues, nil)

	if math.IsNaN(association) || math.IsInf(association, 0) {
		input.Err = io.EOF
		return
	}

	input.Put(SymbolAssociation, association)
}

/*
EffectScales writes the standard deviations of the treatment and target columns
to SymbolTreatmentScale and SymbolTargetScale. A zero-dispersion column reports
io.EOF.
*/
func EffectScales(input *types.Frame) {
	target, hasTarget := input.Get(SymbolTarget)
	treatment, hasTreatment := input.Get(SymbolTreatment)

	if !hasTarget || !hasTreatment {
		input.Err = fmt.Errorf("causal: effect scales require target and treatment roles")
		return
	}

	table, err := readWindow(*input)

	if err != nil {
		input.Err = err
		return
	}

	treatmentScale := stat.StdDev(columnValues(table, int(treatment)), nil)
	targetScale := stat.StdDev(columnValues(table, int(target)), nil)

	if treatmentScale <= 0 || targetScale <= 0 {
		input.Err = io.EOF
		return
	}

	input.Put(SymbolTreatmentScale, treatmentScale)
	input.Put(SymbolTargetScale, targetScale)
}

/*
Percentile writes the interpolated quantile of the treatment column to
SymbolLevel using (n-1) indexing. The requested fraction is read from
SymbolLevel before it is overwritten.
*/
func Percentile(input *types.Frame) {
	treatment, hasTreatment := input.Get(SymbolTreatment)
	fraction, hasFraction := input.Get(SymbolLevel)

	if !hasTreatment || !hasFraction {
		input.Err = fmt.Errorf("causal: percentile requires treatment role and level fraction")
		return
	}

	if fraction < 0 || fraction > 1 || math.IsNaN(fraction) {
		input.Err = fmt.Errorf("causal: percentile fraction must lie in [0, 1]")
		return
	}

	table, err := readWindow(*input)

	if err != nil {
		input.Err = err
		return
	}

	values := columnValues(table, int(treatment))

	if len(values) == 0 {
		input.Err = fmt.Errorf("causal: percentile has no values")
		return
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	count := len(sorted)
	position := float64(count-1) * fraction
	lower := int(math.Floor(position))
	remainder := position - float64(lower)

	if lower >= count-1 {
		input.Put(SymbolLevel, sorted[count-1])
		return
	}

	input.Put(SymbolLevel, sorted[lower]+remainder*(sorted[lower+1]-sorted[lower]))
}

/*
BackdoorEffect writes a kernel-weighted backdoor-adjusted effect to
SymbolBackdoor. The controls are every window column that is neither the target
nor the treatment. It residualizes the target and treatment against those
controls, applies a Gaussian kernel weighting by the standardized control
distance of the current row, and normalizes by the weighted treatment square. A
zero weight sum reports io.EOF.
*/
func BackdoorEffect(input *types.Frame) {
	target, hasTarget := input.Get(SymbolTarget)
	treatment, hasTreatment := input.Get(SymbolTreatment)
	bandwidth, hasBandwidth := input.Get(SymbolBandwidth)

	if !hasTarget || !hasTreatment || !hasBandwidth {
		input.Err = fmt.Errorf("causal: backdoor requires roles and bandwidth")
		return
	}

	if bandwidth <= 0 {
		input.Err = fmt.Errorf("causal: kernel bandwidth must be positive")
		return
	}

	table, err := readWindow(*input)

	if err != nil {
		input.Err = err
		return
	}

	targetValues := columnValues(table, int(target))
	treatmentValues := columnValues(table, int(treatment))
	width := len(table[0])

	var controls []int

	for column := 0; column < width; column++ {
		if column == int(target) || column == int(treatment) {
			continue
		}

		controls = append(controls, column)
	}

	if len(controls) == 0 {
		input.Put(SymbolBackdoor, 0)
		return
	}

	controlColumns := make([][]float64, len(controls))
	controlScales := make([]float64, len(controls))

	for index, control := range controls {
		column := columnValues(table, control)
		controlColumns[index] = column
		controlScales[index] = stat.StdDev(column, nil)

		if controlScales[index] <= 0 {
			input.Err = io.EOF
			return
		}
	}

	residualTarget, err := residualizeColumn(targetValues, controlColumns...)

	if err != nil {
		input.Err = err
		return
	}

	residualTreatment, err := residualizeColumn(treatmentValues, controlColumns...)

	if err != nil {
		input.Err = err
		return
	}

	current := table[len(table)-1]
	numerator := 0.0
	weightSum := 0.0

	for rowIndex, row := range table {
		distanceSum := 0.0

		for controlIndex, feature := range controls {
			delta := (current[feature] - row[feature]) / controlScales[controlIndex]
			distanceSum += delta * delta
		}

		distance := math.Sqrt(distanceSum)
		weight := math.Exp(-distance * distance / (2 * bandwidth * bandwidth))

		numerator += weight * residualTarget[rowIndex] * residualTreatment[rowIndex]
		weightSum += weight * residualTreatment[rowIndex] * residualTreatment[rowIndex]
	}

	if weightSum <= 0 {
		input.Err = io.EOF
		return
	}

	input.Put(SymbolBackdoor, numerator/weightSum)
}

/*
residualizeColumn removes the linear control projection from a column and
returns the residual series, using the in-repo OLS primitive.
*/
func residualizeColumn(target []float64, controls ...[]float64) ([]float64, error) {
	if len(controls) == 0 {
		return append([]float64(nil), target...), nil
	}

	rowCount := len(target)

	for _, control := range controls {
		if len(control) != rowCount {
			return nil, fmt.Errorf("causal: control length mismatch")
		}
	}

	parameterCount := len(controls) + 1

	if rowCount < parameterCount {
		return nil, io.EOF
	}

	design := make([]float64, 0, rowCount*parameterCount)

	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		design = append(design, 1)

		for _, control := range controls {
			design = append(design, control[rowIndex])
		}
	}

	fit := statistic.FitOLS(design, target, parameterCount)

	if !fit.Defined {
		return nil, io.EOF
	}

	residuals := make([]float64, rowCount)

	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		fitted := fit.Coefficients[0]

		for controlIndex, control := range controls {
			fitted += fit.Coefficients[controlIndex+1] * control[rowIndex]
		}

		residuals[rowIndex] = target[rowIndex] - fitted
	}

	return residuals, nil
}

/*
DoExpectationFrame writes the average structural prediction at the intervening
treatment level to SymbolDoExpectation, using the retained window as the
observed control distribution and a linear structural fit.
*/
func DoExpectationFrame(input *types.Frame) {
	target, hasTarget := input.Get(SymbolTarget)
	treatment, hasTreatment := input.Get(SymbolTreatment)
	level, hasLevel := input.Get(SymbolLevel)

	if !hasTarget || !hasTreatment || !hasLevel {
		input.Err = fmt.Errorf("causal: do expectation requires roles and level")
		return
	}

	table, err := readWindow(*input)

	if err != nil {
		input.Err = err
		return
	}

	var controls []int

	for column := 0; column < len(table[0]); column++ {
		if column == int(target) || column == int(treatment) {
			continue
		}

		controls = append(controls, column)
	}

	expectation, err := DoExpectation(
		table, int(target), len(table), int(treatment), level, controls,
	)

	if err != nil {
		input.Err = err
		return
	}

	input.Put(SymbolDoExpectation, expectation)
}

/*
CounterfactualFrame writes the structural counterfactual, its residual noise,
and the uplift to SymbolCounterfactual, SymbolNoise, and SymbolUplift, using the
retained window and the last row as the factual observation.
*/
func CounterfactualFrame(input *types.Frame) {
	target, hasTarget := input.Get(SymbolTarget)
	treatment, hasTreatment := input.Get(SymbolTreatment)
	level, hasLevel := input.Get(SymbolLevel)

	if !hasTarget || !hasTreatment || !hasLevel {
		input.Err = fmt.Errorf("causal: counterfactual requires roles and level")
		return
	}

	table, err := readWindow(*input)

	if err != nil {
		input.Err = err
		return
	}

	if len(table) < 1 || len(table[0]) <= int(target) {
		input.Err = fmt.Errorf("causal: counterfactual requires a factual row with a target")
		return
	}

	actual := table[len(table)-1]

	var features []int

	for column := 0; column < len(table[0]); column++ {
		if column == int(target) {
			continue
		}

		features = append(features, column)
	}

	counterfactual, noise, err := AbductiveCounterfactual(
		table, int(target), len(table), features, true, actual, int(treatment), level,
	)

	if err != nil {
		input.Err = err
		return
	}

	input.Put(SymbolCounterfactual, counterfactual)
	input.Put(SymbolNoise, noise)
	input.Put(SymbolUplift, counterfactual-actual[int(target)])
}

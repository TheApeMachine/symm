package causal

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Stump estimates interventional effects across observational evidence through
decision stumps.
*/
type Stump struct {
	*core.PrimitiveError
	rows       [][]float64
	target     int
	treatment  int
	features   []int
	baseline   float64
	thresholds []float64
	belows     []float64
	aboves     []float64
}

/*
NewStump standardizes decision stumps over observational rows.
*/
func NewStump(
	rows [][]float64,
	target int,
	treatment int,
	features []int,
) *Stump {
	prim := &Stump{
		PrimitiveError: core.NewPrimitiveError(),
		target:         target,
		treatment:      treatment,
	}

	if len(rows) == 0 {
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
		if len(row) != columnCount {
			prim.Error(core.ErrShape)
			return prim
		}

		observations[rowIndex] = slices.Clone(row)
	}

	targetSum := 0.0

	for _, row := range observations {
		targetSum += row[target]
	}

	baseline := targetSum / float64(len(observations))
	residuals := make([]float64, len(observations))

	for rowIndex, row := range observations {
		residuals[rowIndex] = row[target] - baseline
	}

	thresholds := make([]float64, len(featureCols))
	belows := make([]float64, len(featureCols))
	aboves := make([]float64, len(featureCols))

	for featureIndex, column := range featureCols {
		colSum := 0.0

		for _, row := range observations {
			colSum += row[column]
		}

		threshold := colSum / float64(len(observations))
		belowSum := 0.0
		belowCount := 0
		aboveSum := 0.0
		aboveCount := 0

		for rowIndex, row := range observations {
			if row[column] <= threshold {
				belowSum += residuals[rowIndex]
				belowCount++
				continue
			}

			aboveSum += residuals[rowIndex]
			aboveCount++
		}

		thresholds[featureIndex] = threshold

		if belowCount > 0 {
			belows[featureIndex] = belowSum / float64(belowCount)
		}

		if aboveCount > 0 {
			aboves[featureIndex] = aboveSum / float64(aboveCount)
		}
	}

	prim.rows = observations
	prim.features = featureCols
	prim.baseline = baseline
	prim.thresholds = thresholds
	prim.belows = belows
	prim.aboves = aboves

	return prim
}

/*
Next evaluates interventional expectation through decision stumps.
*/
func (op *Stump) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			level := *(*float64)(arriving)

			if len(op.rows) == 0 {
				op.Error(core.ErrDomain)
				return
			}

			total := 0.0

			for _, observation := range op.rows {
				prediction := op.baseline

				for featureIndex, column := range op.features {
					val := observation[column]

					if column == op.treatment {
						val = level
					}

					if val <= op.thresholds[featureIndex] {
						prediction += op.belows[featureIndex]
						continue
					}

					prediction += op.aboves[featureIndex]
				}

				total += prediction
			}

			expectation := total / float64(len(op.rows))
			defined := 1.0

			for value := range data.NewValue(expectation, defined).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}

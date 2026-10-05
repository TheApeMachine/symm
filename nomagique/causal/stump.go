package causal

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Stump estimates interventional effects through an ensemble of decision stumps.
*/
type Stump struct {
	*core.PrimitiveError
	baseline   float64
	columns    []int
	thresholds []float64
	belows     []float64
	aboves     []float64
	treatment  int
	rows       [][]float64
	input      data.Map[string]
	output     data.Map[float64]
}

func NewStump(evidence ...any) *Stump {
	output := data.NewOutputMap()
	output.Values["expectation"] = 0
	output.Values["defined"] = 0

	op := &Stump{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("level", "level"),
		output:         output,
	}

	if len(evidence) < 4 {
		return op
	}

	rows, rowsOK := evidence[0].([][]float64)
	target, targetOK := evidence[1].(int)
	treatment, treatmentOK := evidence[2].(int)
	controls, controlsOK := evidence[3].([]int)

	if !rowsOK || !targetOK || !treatmentOK || !controlsOK {
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

	features := make([]int, 0, len(controls)+1)
	seen := make(map[int]bool, len(controls)+1)

	for _, feature := range append(slices.Clone(controls), treatment) {
		if feature < 0 || feature >= columnCount {
			op.Error(core.ErrShape)
			return op
		}

		if feature == target || seen[feature] {
			continue
		}

		seen[feature] = true
		features = append(features, feature)
	}

	if len(features) == 0 {
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

	targetTotal := 0.0

	for _, row := range copied {
		targetTotal += row[target]
	}

	baseline := targetTotal / float64(len(copied))
	residuals := make([]float64, len(copied))

	for rowIndex, row := range copied {
		residuals[rowIndex] = row[target] - baseline
	}

	op.baseline = baseline
	op.treatment = treatment
	op.rows = copied
	op.columns = make([]int, 0, len(features))
	op.thresholds = make([]float64, 0, len(features))
	op.belows = make([]float64, 0, len(features))
	op.aboves = make([]float64, 0, len(features))

	for _, column := range features {
		columnTotal := 0.0

		for _, row := range copied {
			columnTotal += row[column]
		}

		threshold := columnTotal / float64(len(copied))
		belowSum := 0.0
		belowCount := 0
		aboveSum := 0.0
		aboveCount := 0

		for rowIndex, row := range copied {
			if row[column] <= threshold {
				belowSum += residuals[rowIndex]
				belowCount++
				continue
			}

			aboveSum += residuals[rowIndex]
			aboveCount++
		}

		belowVal := 0.0

		if belowCount > 0 {
			belowVal = belowSum / float64(belowCount)
		}

		aboveVal := 0.0

		if aboveCount > 0 {
			aboveVal = aboveSum / float64(aboveCount)
		}

		op.columns = append(op.columns, column)
		op.thresholds = append(op.thresholds, threshold)
		op.belows = append(op.belows, belowVal)
		op.aboves = append(op.aboves, aboveVal)
	}

	return op
}

func (op *Stump) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if len(op.rows) == 0 {
				op.Error(core.ErrDomain)
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

			expectation := 0.0

			for _, observation := range op.rows {
				prediction := op.baseline

				for index, column := range op.columns {
					value := observation[column]

					if column == op.treatment {
						value = level
					}

					if value <= op.thresholds[index] {
						prediction += op.belows[index]
						continue
					}

					prediction += op.aboves[index]
				}

				expectation += prediction
			}

			op.output.Values["expectation"] = expectation / float64(len(op.rows))
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

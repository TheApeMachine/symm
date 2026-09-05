package learning

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Grid owns current numerical values and a shared two-dimensional co-activation
layout. Rows identify independent input contexts; columns identify quantities
by source and label. These strings are addresses, never classification rules.

Values, Present and Coordinates are borrowed live storage. One owner calls
Step and reads the grid synchronously. Coordinates persist when values change.
Present distinguishes readings in the current update from missing readings,
including previously observed values still retained in Values.
*/
type Grid struct {
	Rows        []string
	Columns     [][2]string
	Values      [][]float64
	Present     [][]bool
	Coordinates []*[2]float64
	Version     uint64

	rowIndex    map[string]int
	versions    []uint64
	columnIndex map[[2]string]int
	baselines   [][]equation.CausalResidual
	activations [][]float64
	qualities   [][]float64
	weights     []float64
	cursor      int
	basis       [gridDirections][]float64
	next        [gridDimensions][]float64
	gram        [gridDirections * gridDirections]float64
	eigenvalues [gridDirections]float64
	work        []float64
	discarded   float64
	regions     []regions
}

/*
NewGrid constructs an empty live grid. Two dimensions are the requested output
geometry; one additional direction admits the next observation before the
rank-two truncation. There is no chosen cluster count or history span.
*/
func NewGrid() *Grid {
	return &Grid{
		rowIndex:    make(map[string]int),
		columnIndex: make(map[[2]string]int),
		cursor:      -1,
	}
}

/*
Step updates one context from its published measurements, then restructures
the shared layout. Other sources retain their latest values, but only newly
published changes contribute movement. Missing readings contribute neither
an observed zero nor a positional force. One update is one sample, not a time
interval. One evidenced, present coordinate advances per call; successive
calls cycle through the present coordinates to spread the optimization work.

Every raw value reaches Values. Layout influence uses change divided by its
adaptive level dispersion, baseline maturity and measurement maturity. Producer SNR
supplies its signal-power fraction. When a producer supplies no noise estimate,
the grid uses its own change-to-dispersion power without declaring the producer's
SNR defined. Both paths retain the baseline and measurement maturity factors.
*/
func (grid *Grid) Step(measurements []*data.Measurement[float64]) error {
	label := ""

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Err != nil {
			return measurement.Err
		}

		if measurement.Label == "" || measurement.Source == "" ||
			(label != "" && label != measurement.Label) {
			return errnie.Err(errnie.Validation, "grid: identified measurements from one context are required", nil)
		}

		label = measurement.Label
	}

	if label == "" {
		return nil
	}

	row, exists := grid.rowIndex[label]

	if !exists {
		row = len(grid.Rows)
		grid.rowIndex[label] = row
		grid.Rows = append(grid.Rows, label)
		grid.versions = append(grid.versions, 0)
		grid.Values = append(grid.Values, make([]float64, len(grid.Columns)))
		grid.Present = append(grid.Present, make([]bool, len(grid.Columns)))
		grid.baselines = append(grid.baselines, make([]equation.CausalResidual, len(grid.Columns)))
		grid.activations = append(grid.activations, make([]float64, len(grid.Columns)))
		grid.qualities = append(grid.qualities, make([]float64, len(grid.Columns)))
	}

	clear(grid.activations[row])
	clear(grid.qualities[row])
	clear(grid.Present[row])

	for _, measurement := range measurements {
		if measurement != nil {
			grid.update(row, measurement)
		}
	}

	if err := grid.restructure(row); err != nil {
		return err
	}

	grid.Version++
	grid.versions[row] = grid.Version

	for column := range grid.Columns {
		grid.weights[column] += grid.qualities[row][column]
	}

	grid.relax(row)

	return nil
}

/* update replaces one source's current readings without clearing its layout. */
func (grid *Grid) update(row int, measurement *data.Measurement[float64]) {
	measurement.Finalize()

	for key, metric := range measurement.Metrics {
		column := grid.column(measurement.Source, key)
		previous := grid.Values[row][column]
		grid.Values[row][column] = metric.Raw
		grid.Present[row][column] = true
		metric.Coordinates = grid.Coordinates[column]
		measurement.Metrics[key] = metric
		baseline := &grid.baselines[row][column]
		baseline.Step(types.Scalar(metric.Raw))
		dispersion := float64(baseline.Dispersion())

		if !baseline.HasPrior() || dispersion <= 0 {
			continue
		}

		movement := (metric.Raw - previous) / dispersion
		snr := movement * movement

		if measurement.SNRDefined {
			snr = measurement.SNR
		}

		quality := float64(baseline.Maturity()) *
			measurement.Maturity * (snr / (1 + snr))
		// Square-root weighting makes each squared activation carry quality
		// once in the accumulated second moment, rather than squaring it.
		grid.activations[row][column] = movement * math.Sqrt(quality)
		grid.qualities[row][column] = quality
	}
}

/* column admits one quantity and extends storage only when the grid grows. */
func (grid *Grid) column(source, key string) int {
	identity := [2]string{source, key}
	column, exists := grid.columnIndex[identity]

	if exists {
		return column
	}

	column = len(grid.Columns)
	grid.columnIndex[identity] = column
	grid.Columns = append(grid.Columns, identity)
	grid.Coordinates = append(grid.Coordinates, new([2]float64))
	grid.weights = append(grid.weights, 0)

	for row := range grid.Rows {
		grid.Values[row] = append(grid.Values[row], 0)
		grid.Present[row] = append(grid.Present[row], false)
		grid.baselines[row] = append(grid.baselines[row], equation.CausalResidual{})
		grid.activations[row] = append(grid.activations[row], 0)
		grid.qualities[row] = append(grid.qualities[row], 0)
	}

	for direction := range gridDirections {
		grid.basis[direction] = append(grid.basis[direction], 0)
	}

	for dimension := range gridDimensions {
		grid.next[dimension] = append(grid.next[dimension], 0)
	}

	return column
}

/*
CovarianceError bounds the spectral error of the internal signed second-moment
sketch per committed sample, before deriving distances for coordinate movement.
Each discarded rank-one PSD component has norm equal to its squared singular
value, so their sum bounds the accumulated loss by the triangle inequality.
Proximity in a lossy two-dimensional layout is not proof of identical profiles.
*/
func (grid *Grid) CovarianceError() float64 {
	if grid.Version == 0 {
		return 0
	}

	return grid.discarded / float64(grid.Version)
}

package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/adaptive"
)

/*
PearlInput is one keyed numeric observation for causal evaluation.
*/
type PearlInput struct {
	Key          string
	Row          []float64
	Inverted     bool
	Contagion    float64
	Condition    float64
	Intervention float64
}

/*
PearlSample retains aligned numeric causal rows by key.
*/
type PearlSample struct {
	windows map[string]*pearlWindow
}

/*
PearlSampleOutput is the current causal row and retained table.
*/
type PearlSampleOutput struct {
	Key  string
	Row  []float64
	Rows [][]float64
}

type pearlWindow struct {
	rows      [][]float64
	variances []*adaptive.Variance
	capacity  int
}

/*
NewPearlSample returns a keyed numeric causal sampler.
*/
func NewPearlSample(configs ...PearlConfig) *PearlSample {
	return &PearlSample{
		windows: map[string]*pearlWindow{},
	}
}

/*
Measure observes one keyed numeric causal row.
*/
func (pearlSample *PearlSample) Measure(
	input PearlInput,
) (PearlSampleOutput, bool, error) {
	if input.Key == "" {
		return PearlSampleOutput{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"pearl-sample: key required",
			nil,
		))
	}

	if len(input.Row) == 0 {
		return PearlSampleOutput{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"pearl-sample: row required",
			nil,
		))
	}

	row := make([]float64, 0, len(input.Row))

	for _, value := range input.Row {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return PearlSampleOutput{}, false, errnie.Error(errnie.Err(
				errnie.Validation,
				"pearl-sample: row contains non-finite value",
				nil,
			))
		}

		row = append(row, value)
	}

	window := pearlSample.window(input.Key, len(row))

	if len(window.rows) > 0 && len(window.rows[0]) != len(row) {
		return PearlSampleOutput{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"pearl-sample: row width %d differs from retained width %d",
				len(row),
				len(window.rows[0]),
			),
			nil,
		))
	}

	for index, value := range row {
		if len(window.variances) == index {
			window.variances = append(window.variances, adaptive.NewVariance())
		}

		if _, err := window.variances[index].Measure(value); err != nil {
			return PearlSampleOutput{}, false, err
		}
	}

	return pearlSample.append(input.Key, row, window), pearlSample.ready(window), nil
}

func (pearlSample *PearlSample) append(
	key string,
	row []float64,
	window *pearlWindow,
) PearlSampleOutput {
	window.rows = append(window.rows, row)
	window.trim()

	return PearlSampleOutput{
		Key:  key,
		Row:  row,
		Rows: window.rows,
	}
}

func (window *pearlWindow) trim() {
	capacity := window.capacity

	if len(window.rows) <= capacity {
		return
	}

	drop := len(window.rows) - capacity
	copy(window.rows, window.rows[drop:])
	clear(window.rows[capacity:])
	window.rows = window.rows[:capacity]
}

func (pearlSample *PearlSample) ready(window *pearlWindow) bool {
	for _, variance := range window.variances {
		if variance.Count() < 2 {
			return false
		}
	}

	return len(window.variances) > 0
}

func (pearlSample *PearlSample) window(key string, rowWidth int) *pearlWindow {
	window, ok := pearlSample.windows[key]

	if ok {
		return window
	}

	window = &pearlWindow{capacity: causalMomentParameterCount(rowWidth)}
	pearlSample.windows[key] = window

	return window
}

func causalMomentParameterCount(rowWidth int) int {
	return 1 + rowWidth + rowWidth*(rowWidth+1)/2
}

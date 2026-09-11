package grid

import (
	"fmt"
	"math"
	"sync"
	"time"

	"iter"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

const (
	gridDimensions = 2
)

/*
Space owns current numerical values and a shared two-dimensional co-activation
layout. Rows identify independent input contexts; columns identify quantities
by source and label. These strings are addresses, never classification rules.

Values, Present and Coordinates are borrowed live storage. One owner calls
Step and reads the grid synchronously. Coordinates persist when values change.
Present distinguishes readings in the current update from missing readings,
including previously observed values still retained in Values.
*/
type Space struct {
	core.Base[[]*data.Measurement[float64], Impulse]
	mu           sync.RWMutex
	Rows         []string
	Columns      [][2]string
	Values       [][]float64
	Present      [][]bool
	Coordinates  []*[2]float64
	Version      uint64
	UpdatedLabel string
	Formed       bool

	rowIndex    map[string]int
	versions    []uint64
	columnIndex map[[2]string]int
	baselines   [][]*adaptive.Baseline
	activations [][]float64
	qualities   [][]float64
	weights     []float64
	cursor      int
	window      *window
	graph       [][]affinity
	regions     regions
	moved       bool
}

/*
NewSpace constructs an empty live grid over the declared window span. Two
dimensions are the requested output geometry. There is no chosen cluster count.
*/
func NewSpace() *Space {
	return NewSpaceWithWindow(DefaultWindowBins)
}

/*
DefaultWindowBins is the declared span of recent observation bins the affinity
channels are measured over.

It is a declared selector rather than a derived quantity, in the same sense the
episode discovery policy declares its thresholds: how much of the past still
describes the present is a modelling choice, and the honest thing is to state
it and report it alongside what it produced, not to dress it up as measured.
*/
const DefaultWindowBins = 64

/* NewSpaceWithWindow constructs a live grid over a declared window span. */
func NewSpaceWithWindow(bins int) *Space {
	return &Space{
		rowIndex:    make(map[string]int),
		columnIndex: make(map[[2]string]int),
		cursor:      -1,
		window:      newWindow(bins),
	}
}

/*
Step updates one context from its published measurements. During calibration,
movement enters the retained feature window. Formation then solves that fixed
affinity objective one coordinate per observed quantity and retains its partition. Later
inputs update activation only. Other sources retain their latest values, but
only newly published changes contribute activation. A missing reading is never
an observed zero. One update is one sample, not a time interval.

Every raw value reaches Values. Layout influence uses change divided by its
adaptive level dispersion, baseline maturity and measurement maturity. Producer SNR
supplies its signal-power fraction. When a producer supplies no noise estimate,
the grid uses its own change-to-dispersion power without declaring the producer's
SNR defined. Both paths retain the baseline and measurement maturity factors.
*/
func (grid *Space) Next(
	in iter.Seq[core.Primitive[[]*data.Measurement[float64], []*data.Measurement[float64]]],
) iter.Seq[core.Primitive[Impulse, Impulse]] {
	return func(yield func(core.Primitive[Impulse, Impulse]) bool) {
		for arriving := range in {
			measurements := arriving.Read()

			if len(measurements) == 0 {
				continue
			}

			if err := grid.Step(measurements); err != nil {
				grid.Error(err)
				return
			}

			at, from := observed(measurements)
			impulse, err := grid.Impulse(grid.UpdatedLabel, at, from)

			if err != nil {
				grid.Error(err)
				return
			}

			if !yield(grid.Carrier(impulse)) {
				return
			}
		}
	}
}

/*
observed is when one context's readings were taken and how far back the
earliest of them looks. Both come from the readings themselves: an impulse is
positioned where its evidence was observed, never where it was processed.
*/
func observed(measurements []*data.Measurement[float64]) (time.Time, time.Time) {
	var at, from time.Time

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.At.After(at) {
			at = measurement.At
		}

		if !measurement.From.IsZero() && (from.IsZero() || measurement.From.Before(from)) {
			from = measurement.From
		}
	}

	return at, from
}

func (grid *Space) Step(measurements []*data.Measurement[float64]) error {
	grid.mu.Lock()
	defer grid.mu.Unlock()

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
		grid.baselines = append(grid.baselines, make([]*adaptive.Baseline, len(grid.Columns)))
		grid.activations = append(grid.activations, make([]float64, len(grid.Columns)))
		grid.qualities = append(grid.qualities, make([]float64, len(grid.Columns)))
	}

	clear(grid.activations[row])
	clear(grid.qualities[row])
	clear(grid.Present[row])

	for _, measurement := range measurements {
		if measurement != nil {
			if err := grid.update(row, measurement); err != nil {
				return err
			}
		}
	}

	if grid.window.covered() {
		grid.window.close()
	}

	grid.Version++
	grid.UpdatedLabel = label
	grid.versions[row] = grid.Version

	if grid.graph == nil {
		for column := range grid.Columns {
			grid.weights[column] += grid.qualities[row][column]
		}
	}

	for _, observed := range grid.Present[row] {
		if grid.Formed {
			break
		}

		if !observed {
			continue
		}
		grid.form()

		if grid.graph == nil {
			break
		}
	}

	return nil
}

/*
update replaces one source's current readings without clearing its layout, and
records their movement in the window's open bin.
*/
func (grid *Space) update(row int, measurement *data.Measurement[float64]) error {
	measurement.Finalize()
	if measurement.Err != nil {
		return measurement.Err
	}

	for key, metric := range measurement.Metrics {
		column := grid.columnLocked(measurement.Source, key)
		previous := grid.Values[row][column]
		grid.Values[row][column] = metric.Raw
		grid.Present[row][column] = true
		metric.Coordinates = grid.Coordinates[column]
		measurement.Metrics[key] = metric
		baseline := grid.baselines[row][column]
		if baseline == nil {
			baseline = adaptive.NewBaseline(adaptive.NewWindow())
			grid.baselines[row][column] = baseline
		}
		reading := baseline.Observe(metric.Raw)
		dispersion, hasPrior, maturity := reading.Dispersion, reading.HasPrior, reading.Maturity

		if !hasPrior || dispersion <= 0 {
			continue
		}

		movement := (metric.Raw - previous) / dispersion

		if math.IsNaN(movement) || math.IsInf(movement, 0) {
			return grid.numericFailure(row, column, metric.Raw, previous, dispersion)
		}
		snr := movement * movement

		if measurement.SNRDefined {
			snr = measurement.SNR
		}

		snrFactor := 0.0

		if snr > 0 {
			snrFactor = 1.0

			if snr < 1e12 {
				snrFactor = snr / (1 + snr)
			}
		}

		quality := maturity *
			measurement.Maturity * snrFactor
		// Square-root weighting makes each squared activation carry quality
		// once in the accumulated second moment, rather than squaring it.
		grid.activations[row][column] = movement * math.Sqrt(quality)
		grid.qualities[row][column] = quality
		if grid.graph == nil {
			grid.window.observe(column, grid.activations[row][column])
		}
	}
	return nil
}

/*
numericFailure reports a movement that arithmetic could not represent, with the
readings that produced it.

Both observations can be representable while their difference is not. That is a
numerical failure and it is fatal: writing a value that is not a number into the
window would silently poison every relationship the quantity takes part in,
which is worse than stopping. It is never a missing-value fallback — an absent
reading has its own representation and does not come through here.
*/
func (grid *Space) numericFailure(row, column int, raw, previous, dispersion float64) error {
	detail := fmt.Sprintf(
		"grid: quantity movement is not finite; context=%q committed_version=%d; source=%q metric=%q raw=%g previous=%g dispersion=%g",
		grid.Rows[row], grid.Version,
		grid.Columns[column][0], grid.Columns[column][1], raw, previous, dispersion,
	)

	if baseline := grid.baselines[row][column]; baseline != nil {
		detail += fmt.Sprintf(" baseline=%+v", baseline.Reading)
	}

	return errnie.Error(errnie.Err(errnie.Internal, detail, nil))
}

/* Column admits one quantity and extends storage only when the grid grows. */
func (grid *Space) Column(source, key string) int {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	return grid.columnLocked(source, key)
}

func (grid *Space) columnLocked(source, key string) int {
	identity := [2]string{source, key}
	column, exists := grid.columnIndex[identity]

	if exists {
		return column
	}

	column = len(grid.Columns)
	// A new quantity changes the schema of the partition. Ordinary values,
	// quiet observations and missing producers never invalidate formation.
	if grid.graph != nil {
		grid.graph = nil
		grid.Formed = false
		grid.cursor, grid.moved = -1, false
	}
	grid.columnIndex[identity] = column
	grid.Columns = append(grid.Columns, identity)
	grid.Coordinates = append(grid.Coordinates, new([2]float64))
	grid.weights = append(grid.weights, 0)

	for row := range grid.Rows {
		grid.Values[row] = append(grid.Values[row], 0)
		grid.Present[row] = append(grid.Present[row], false)
		grid.baselines[row] = append(grid.baselines[row], nil)
		grid.activations[row] = append(grid.activations[row], 0)
		grid.qualities[row] = append(grid.qualities[row], 0)
	}

	grid.window.columns(len(grid.Columns))

	return column
}

/* PreseedColumns allocates storage for known sources and metrics upfront. */
func (grid *Space) PreseedColumns(sources map[string][]string) {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	for source, keys := range sources {
		for _, key := range keys {
			grid.columnLocked(source, key)
		}
	}
}

/*
Reset clears observation-local baselines between independent historical tapes.
The affinity calibration, settled coordinates and region membership survive.
*/
func (grid *Space) Reset() {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	// An unfinished calibration bin cannot pair observations from unrelated
	// tapes. Completed bins still retain their observed relationships.
	clear(grid.window.open)
	clear(grid.window.observed)

	for row := range grid.Rows {
		clear(grid.Values[row])
		clear(grid.Present[row])
		clear(grid.baselines[row])
		clear(grid.activations[row])
		clear(grid.qualities[row])
	}
}

/*
Support reports how many observation bins one quantity was actually seen in,
against the window's declared span.

Proximity in a two-dimensional layout is not proof of identical profiles, and a
relationship measured over two shared bins is not the claim a relationship
measured over sixty is. This is how a reader tells those apart.
*/
func (grid *Space) Support(source, key string) (int, int) {
	grid.mu.RLock()
	defer grid.mu.RUnlock()

	column, exists := grid.columnIndex[[2]string{source, key}]

	if !exists {
		return 0, grid.window.capacity
	}

	return grid.window.support(column), grid.window.capacity
}

/* Label returns the most recently updated context label under read lock. */
func (grid *Space) Label() string {
	grid.mu.RLock()
	defer grid.mu.RUnlock()

	return grid.UpdatedLabel
}

/*
QuantitySnapshot is a point-in-time capture of one quantity for telemetry.
*/
type QuantitySnapshot struct {
	Source   string
	Label    string
	X        float64
	Y        float64
	Value    float64
	Activity float64
	Quality  float64
	Present  bool
}

/*
MarketSnapshot provides an atomic point-in-time read of all quantities and regions
for an instrument context without causing data races with concurrent Step calls.
*/
func (grid *Space) MarketSnapshot(label string) ([]QuantitySnapshot, []Region, uint64, error) {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	row, exists := grid.rowIndex[label]

	if !exists {
		return nil, nil, 0, errnie.Error(errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil))
	}

	quantities := make([]QuantitySnapshot, 0, len(grid.Columns))

	for column, identity := range grid.Columns {
		coordinate := grid.Coordinates[column]
		x, y := 0.0, 0.0

		if coordinate != nil {
			x, y = coordinate[0], coordinate[1]
		}

		value := 0.0

		if row < len(grid.Values) && column < len(grid.Values[row]) {
			value = grid.Values[row][column]
		}

		act := 0.0

		if row < len(grid.activations) && column < len(grid.activations[row]) {
			act = grid.activations[row][column]
		}

		qual := 0.0

		if row < len(grid.qualities) && column < len(grid.qualities[row]) {
			qual = grid.qualities[row][column]
		}

		present := false

		if row < len(grid.Present) && column < len(grid.Present[row]) {
			present = grid.Present[row][column]
		}

		quantities = append(quantities, QuantitySnapshot{
			Source:   identity[0],
			Label:    identity[1],
			X:        x,
			Y:        y,
			Value:    value,
			Activity: act,
			Quality:  qual,
			Present:  present,
		})
	}

	regions, version, err := grid.regionsLocked(label)

	if err != nil {
		return quantities, nil, version, err
	}

	return quantities, regions, version, nil
}

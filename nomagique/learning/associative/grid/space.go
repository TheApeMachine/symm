/*
Space owns current numerical values and a shared two-dimensional co-activation
layout. Rows identify independent input contexts; columns identify quantities
by source and label. These strings are addresses, never classification rules.

Space is a streaming Primitive over an unsafe.Pointer wire. Commands
discriminate one intent each: stepping a context's published measurements,
reading the impulse, the regions, a market snapshot, the grid's state,
preseeding columns or resetting observation-local state. Payloads are plain
data types with exported fields only and no methods.
*/
package grid

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
DefaultWindowBins is the declared span of recent observation bins the affinity
channels are measured over.

It is a declared selector rather than a derived quantity, in the same sense the
episode discovery policy declares its thresholds: how much of the past still
describes the present is a modelling choice, and the honest thing is to state
it and report it alongside what it produced, not to dress it up as measured.
*/
const DefaultWindowBins = 64

/*
Command discriminates one grid intent. Exactly one field is set; any other
shape is a failure recorded in Error and ends the stream.
*/
type Command struct {
	Step     []*data.Measurement[float64]
	Impulse  *ImpulseQuery
	Snapshot *SnapshotQuery
	Regions  *RegionsQuery
	State    *State
	Preseed  map[string][]string
	Reset    *ResetSignal
}

/*
Result is one command's answer: the impulse a step or impulse query produced,
the snapshot's quantity rows, the measured regions, and the grid state a state
query read.
*/
type Result struct {
	Impulse    Impulse
	Quantities []QuantitySnapshot
	Regions    []Region
	Version    uint64
	Rows       []string
	Columns    int
	Formed     bool
	Updated    string
}

/* ImpulseQuery asks for the current activation sequence of one context. */
type ImpulseQuery struct {
	Label    string
	At, From time.Time
}

/* SnapshotQuery asks for an atomic point-in-time read of one context. */
type SnapshotQuery struct {
	Label string
}

/* RegionsQuery asks for the active region projection of one context. */
type RegionsQuery struct {
	Label string
}

/* State asks for the grid's labels, width, formation and version. */
type State struct{}

/* Reset clears observation-local state between independent tapes. */
type ResetSignal struct{}

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
Space is the grid Primitive. It owns its live storage, its baselines, its
retained window, its calibrated affinity graph and its region partition.
One owner drives it through commands and reads results synchronously.
*/
type Space struct {
	err         error
	out         Result
	mu          sync.RWMutex
	rows        []string
	columns     [][2]string
	values      [][]float64
	present     [][]bool
	coordinates []*[2]float64
	version     uint64
	updated     string
	formed      bool

	rowIndex    map[string]int
	versions    []uint64
	columnIndex map[[2]string]int
	finalizer   core.Primitive
	baselines   [][]core.Primitive
	zscores     [][]float64
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
func NewSpace(bins ...int) core.Primitive {
	span := DefaultWindowBins

	if len(bins) > 0 {
		span = bins[0]
	}

	return &Space{
		rowIndex:    make(map[string]int),
		columnIndex: make(map[[2]string]int),
		finalizer:   data.NewFinalizer[float64](),
		cursor:      -1,
		window:      newWindow(span),
	}
}

/*
Next executes each arriving command and yields its result. An invalid command
is recorded in Error and ends the stream.
*/
func (op *Space) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*Command)(arriving)
			result, err := op.execute(command)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Space) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (op *Space) execute(command *Command) (Result, error) {
	intents := 0

	if command.Step != nil {
		intents++
	}

	if command.Impulse != nil {
		intents++
	}

	if command.Snapshot != nil {
		intents++
	}

	if command.Regions != nil {
		intents++
	}

	if command.State != nil {
		intents++
	}

	if command.Preseed != nil {
		intents++
	}

	if command.Reset != nil {
		intents++
	}

	if intents != 1 {
		return Result{}, fmt.Errorf(
			"%w: grid: command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Step != nil {
		return op.stepCommand(command.Step)
	}

	if command.Impulse != nil {
		impulse, err := op.impulse(command.Impulse.Label, command.Impulse.At, command.Impulse.From)

		if err != nil {
			return Result{}, err
		}

		return Result{Impulse: impulse}, nil
	}

	if command.Snapshot != nil {
		return op.snapshot(command.Snapshot.Label)
	}

	if command.Regions != nil {
		measured, version, err := op.regionsOf(command.Regions.Label)

		if err != nil {
			return Result{}, err
		}

		return Result{Regions: measured, Version: version}, nil
	}

	if command.State != nil {
		return op.state(), nil
	}

	if command.Preseed != nil {
		return Result{}, op.preseed(command.Preseed)
	}

	return Result{}, op.reset()
}

/*
stepCommand updates one context from its published measurements and reads the
impulse that update produced, positioned where its evidence was observed.

Every raw value reaches the values store. Layout influence uses change divided
by its adaptive level dispersion, baseline maturity and measurement maturity.
Producer SNR supplies its signal-power fraction. When a producer supplies no
noise estimate, the grid uses its own change-to-dispersion power without
declaring the producer's SNR defined. Both paths retain the baseline and
measurement maturity factors.
*/
func (op *Space) stepCommand(measurements []*data.Measurement[float64]) (Result, error) {
	if len(measurements) == 0 {
		return Result{}, fmt.Errorf(
			"%w: grid: step requires identified measurements from one context",
			core.ErrShape,
		)
	}

	if err := op.step(measurements); err != nil {
		return Result{}, err
	}

	at, from := observed(measurements)
	impulse, err := op.impulse(op.updated, at, from)

	if err != nil {
		return Result{}, err
	}

	return Result{Impulse: impulse}, nil
}

/*
step commits one context's measurements. During calibration, movement enters
the retained feature window. Formation then solves that fixed affinity
objective one coordinate per observed quantity and retains its partition.
Later inputs update activation only. Other sources retain their latest values,
but only newly published changes contribute activation. A missing reading is
never an observed zero. One update is one sample, not a time interval.
*/
func (op *Space) step(measurements []*data.Measurement[float64]) error {
	op.mu.Lock()
	defer op.mu.Unlock()

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

	row, exists := op.rowIndex[label]

	if !exists {
		row = len(op.rows)
		op.rowIndex[label] = row
		op.rows = append(op.rows, label)
		op.versions = append(op.versions, 0)
		op.values = append(op.values, make([]float64, len(op.columns)))
		op.present = append(op.present, make([]bool, len(op.columns)))
		op.baselines = append(op.baselines, make([]core.Primitive, len(op.columns)))
		op.zscores = append(op.zscores, make([]float64, len(op.columns)))
		op.activations = append(op.activations, make([]float64, len(op.columns)))
		op.qualities = append(op.qualities, make([]float64, len(op.columns)))
	}

	clear(op.activations[row])
	clear(op.qualities[row])
	clear(op.present[row])

	for _, measurement := range measurements {
		if measurement != nil {
			if err := op.update(row, measurement); err != nil {
				return err
			}
		}
	}

	if op.window.covered() {
		op.window.close()
	}

	op.version++
	op.updated = label
	op.versions[row] = op.version

	if op.graph == nil {
		for column := range op.columns {
			op.weights[column] += op.qualities[row][column]
		}
	}

	for _, observedValue := range op.present[row] {
		if op.formed {
			break
		}

		if !observedValue {
			continue
		}
		op.form()

		if op.graph == nil {
			break
		}
	}

	return nil
}

/*
update replaces one source's current readings without clearing its layout, and
records their movement in the window's open bin.
*/
func (op *Space) update(row int, measurement *data.Measurement[float64]) error {
	for range op.finalizer.Next(transport.NewValues(measurement).Next(nil)) {
	}

	if err := op.finalizer.Error(); err != nil {
		return err
	}

	if measurement.Err != nil {
		return measurement.Err
	}

	for key, metric := range measurement.Metrics {
		column := op.columnLocked(measurement.Source, key)
		previous := op.values[row][column]
		op.values[row][column] = metric.Raw
		op.present[row][column] = true
		metric.Coordinates = op.coordinates[column]
		measurement.Metrics[key] = metric
		baseline := op.baselines[row][column]

		if baseline == nil {
			baseline = adaptive.NewBaseline(adaptive.NewWindow())
			op.baselines[row][column] = baseline
		}

		var reading adaptive.BaselineReading

		for out := range baseline.Next(transport.NewValues(metric.Raw).Next(nil)) {
			reading = *(*adaptive.BaselineReading)(out)
		}

		op.zscores[row][column] = reading.ZScore
		dispersion, hasPrior, maturity := reading.Dispersion, reading.HasPrior, reading.Maturity

		if !hasPrior || dispersion <= 0 {
			continue
		}

		movement := (metric.Raw - previous) / dispersion

		if math.IsNaN(movement) || math.IsInf(movement, 0) {
			return op.numericFailure(row, column, metric.Raw, previous, dispersion)
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
		op.activations[row][column] = movement * math.Sqrt(quality)
		op.qualities[row][column] = quality

		if op.graph == nil {
			op.window.observe(column, op.activations[row][column])
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
func (op *Space) numericFailure(row, column int, raw, previous, dispersion float64) error {
	detail := fmt.Sprintf(
		"grid: quantity movement is not finite; context=%q committed_version=%d; source=%q metric=%q raw=%g previous=%g dispersion=%g",
		op.rows[row], op.version,
		op.columns[column][0], op.columns[column][1], raw, previous, dispersion,
	)

	detail += fmt.Sprintf(" baseline=%+v", op.zscores[row][column])

	return errnie.Error(errnie.Err(errnie.Internal, detail, nil))
}

/* column admits one quantity and extends storage only when the grid grows. */
func (op *Space) column(source, key string) int {
	op.mu.Lock()
	defer op.mu.Unlock()

	return op.columnLocked(source, key)
}

func (op *Space) columnLocked(source, key string) int {
	identity := [2]string{source, key}
	column, exists := op.columnIndex[identity]

	if exists {
		return column
	}

	column = len(op.columns)
	// A new quantity changes the schema of the partition. Ordinary values,
	// quiet observations and missing producers never invalidate formation.
	if op.graph != nil {
		op.graph = nil
		op.formed = false
		op.cursor, op.moved = -1, false
	}
	op.columnIndex[identity] = column
	op.columns = append(op.columns, identity)
	op.coordinates = append(op.coordinates, new([2]float64))
	op.weights = append(op.weights, 0)

	for row := range op.rows {
		op.values[row] = append(op.values[row], 0)
		op.present[row] = append(op.present[row], false)
		op.baselines[row] = append(op.baselines[row], nil)
		op.zscores[row] = append(op.zscores[row], 0)
		op.activations[row] = append(op.activations[row], 0)
		op.qualities[row] = append(op.qualities[row], 0)
	}

	op.window.columns(len(op.columns))

	return column
}

/* preseed allocates storage for known sources and metrics upfront. */
func (op *Space) preseed(sources map[string][]string) error {
	op.mu.Lock()
	defer op.mu.Unlock()

	for source, keys := range sources {
		for _, key := range keys {
			op.columnLocked(source, key)
		}
	}

	return nil
}

/*
reset clears observation-local baselines between independent historical tapes.
The affinity calibration, settled coordinates and region membership survive.
*/
func (op *Space) reset() error {
	op.mu.Lock()
	defer op.mu.Unlock()

	// An unfinished calibration bin cannot pair observations from unrelated
	// tapes. Completed bins still retain their observed relationships.
	clear(op.window.open)
	clear(op.window.observed)

	for row := range op.rows {
		clear(op.values[row])
		clear(op.present[row])
		clear(op.baselines[row])
		clear(op.zscores[row])
		clear(op.activations[row])
		clear(op.qualities[row])
	}

	return nil
}

/*
state reads the grid's labels, width, formation and version.
*/
func (op *Space) state() Result {
	op.mu.RLock()
	defer op.mu.RUnlock()

	return Result{
		Rows:    slices.Clone(op.rows),
		Columns: len(op.columns),
		Formed:  op.formed,
		Version: op.version,
		Updated: op.updated,
	}
}

/*
snapshot provides an atomic point-in-time read of all quantities and regions
for an instrument context without causing data races with concurrent steps.
*/
func (op *Space) snapshot(label string) (Result, error) {
	op.mu.Lock()
	defer op.mu.Unlock()

	row, exists := op.rowIndex[label]

	if !exists {
		return Result{}, errnie.Error(errnie.Err(errnie.NotFound, "grid: unknown context "+label, nil))
	}

	quantities := make([]QuantitySnapshot, 0, len(op.columns))

	for column, identity := range op.columns {
		coordinate := op.coordinates[column]
		x, y := 0.0, 0.0

		if coordinate != nil {
			x, y = coordinate[0], coordinate[1]
		}

		value := 0.0

		if row < len(op.values) && column < len(op.values[row]) {
			value = op.values[row][column]
		}

		act := 0.0

		if row < len(op.activations) && column < len(op.activations[row]) {
			act = op.activations[row][column]
		}

		qual := 0.0

		if row < len(op.qualities) && column < len(op.qualities[row]) {
			qual = op.qualities[row][column]
		}

		present := false

		if row < len(op.present) && column < len(op.present[row]) {
			present = op.present[row][column]
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

	measured, version, err := op.regionsLocked(label)

	if err != nil {
		return Result{}, err
	}

	return Result{Quantities: quantities, Regions: measured, Version: version}, nil
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

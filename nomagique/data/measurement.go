package data

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
MetricEntry stores a single metric by its producer key.
*/
type MetricEntry struct {
	Key    string  `json:"key"`
	Metric *Metric `json:"metric"`
	Err    error   `json:"error"`
}

/*
StringEntry stores a key-value pair for metadata and provenance.
*/
type StringEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Err   error  `json:"error"`
}

const (
	MetadataSupport       = "support"
	MetadataDivergence    = "divergence"
	MetadataNoiseVariance = "noise_variance"
	MetadataReferencePeer = "reference_peer"
)

/*
CrossMember is one peer member state used in cross-sectional calculations.
*/
type CrossMember struct {
	Label  string
	Change float64
	At     time.Time
	From   time.Time
}

/*
Measurement is the native data type in nomagique.

Transferability: when creating a new Measurement using the allocator,
certain properties must transfer over, so they can act as their own
prior when forming new Measurements.
*/
type Measurement struct {
	ID         uint32         // Unique ID (uuid).
	Epoch      int64          // Set once at system start, used for snapshot identification.
	Label      string         // Symbol (e.g. BTC/USD), or other logical label.
	Source     string         // Original owner, which system produced this Measurement.
	SeqIdx     int64          // Event sequence index, used to replay in exact order.
	Tick       int64          // Market tick, used to sync to 1 single tick in time.
	Timestamp  int64          // System nanosecond timestamp (UTC).
	At         time.Time      // Venue timestamp (UTC).
	From       time.Time      // Window from as venue timestamp (UTC).
	maturity   float64        // Maturity of the Measurement.
	samples    int64          // Observations accumulated in the current regime.
	energy     float64        // Total metric excitation energy (sum of z^2).
	prediction float64        // Expected energy baseline from prior.
	err        error          // Error if any.
	metrics    []*MetricEntry // Metrics owned by this Measurement.
	metadata   []*StringEntry // Metadata owned by this Measurement.
	peers      []*Measurement // Peers, used to group related Measurements.
}

/*
NewMeasurement creates a new Measurement.
*/
func NewMeasurement(
	epoch int64,
	label string,
	source string,
	at, from time.Time,
	metadata ...*StringEntry,
) *Measurement {
	tick := system.Tick.Load()

	if source == "spot:trade" || source == "spot:touch" {
		tick = system.Tick.Add(1)
	}

	return &Measurement{
		Epoch:     epoch,
		Label:     label,
		Source:    source,
		SeqIdx:    system.SeqIdx.Add(1),
		Tick:      tick,
		Timestamp: time.Now().UnixNano(),
		At:        at,
		From:      from,
		metrics:   make([]*MetricEntry, 0),
		metadata:  metadata,
		peers:     make([]*Measurement, 0),
	}
}

/*
Next instantiates the source's Measurement derived from the current instance
or the supplied prior. If prior is nil, the current Measurement is used as the prior.
*/
func (measurement *Measurement) Next(
	prior *Measurement,
	values map[string]float64,
	metadata ...*StringEntry,
) *Measurement {
	base := measurement

	if prior != nil {
		base = prior
	}

	if base == nil || base.valid() == nil {
		return nil
	}

	source := base.Source

	if measurement != nil && measurement.Source != "" {
		source = measurement.Source
	}

	meta := slices.Clone(base.metadata)

	for _, entry := range metadata {
		if entry == nil {
			continue
		}

		index := slices.IndexFunc(meta, func(item *StringEntry) bool {
			return item != nil && item.Key == entry.Key
		})

		if index >= 0 {
			meta[index] = entry
			continue
		}

		meta = append(meta, entry)
	}

	next := NewMeasurement(
		base.Epoch,
		base.Label,
		source,
		base.At,
		base.From,
		meta...,
	)

	next.Timestamp = time.Now().UnixNano()

	if prior != nil {
		if !prior.At.IsZero() {
			next.At = prior.At
		}

		next.Tick = prior.Tick
	}

	if measurement != nil && measurement.locked() {
		next.peers = append(next.peers, measurement.peers...)
		next.samples = measurement.samples + 1
		next.prediction = measurement.prediction

		if measurement.samples > 0 {
			next.prediction = measurement.prediction + (1.0/float64(
				measurement.samples,
			))*(measurement.energy-measurement.prediction)
		}
	}

	if measurement == nil || !measurement.locked() {
		next.samples = 1

		if prior != nil {
			next.peers = append(next.peers, prior.peers...)
		}
	}

	next.copyMetrics(source, values, measurement, prior)
	next.finalize()
	return next
}

func (measurement *Measurement) entry(key string) *MetricEntry {
	for _, metricEntry := range measurement.metrics {
		if metricEntry != nil && metricEntry.Key == key {
			return metricEntry
		}
	}

	return nil
}

func (measurement *Measurement) copyMetrics(
	source string, values map[string]float64, prior *Measurement, trigger *Measurement,
) {
	if len(values) == 0 && prior == nil {
		return
	}

	written := make(map[string]struct{}, len(values))

	for _, key := range slices.Sorted(maps.Keys(values)) {
		val := values[key]
		written[key] = struct{}{}
		unit, timescale := GetDimension(source, key)
		var priorEntry *MetricEntry

		if prior != nil {
			priorEntry = prior.entry(key)
		}

		if priorEntry == nil && trigger != nil {
			priorEntry = trigger.entry(key)
		}

		if priorEntry != nil && priorEntry.Metric != nil {
			if unit == "" || unit == UnitNone {
				unit = priorEntry.Metric.unit
			}

			if timescale == "" || timescale == TimescaleNone {
				timescale = priorEntry.Metric.timescale
			}
		}

		metric := NewMetric(key, val, unit, timescale)

		if priorEntry != nil && priorEntry.Metric != nil {
			metric.setSNM(priorEntry.Metric.raw)
		}

		measurement.metrics = append(measurement.metrics, &MetricEntry{
			Key:    key,
			Metric: metric,
		})
	}
}

/*
Prefix returns the prefix for the given key.
*/
func (measurement *Measurement) Prefix() string {
	return fmt.Sprintf(
		"%s/%s/%019d/%019d/%019d/%019d.bin",
		measurement.Source,
		measurement.Label,
		measurement.Epoch,
		measurement.Tick,
		measurement.SeqIdx,
		measurement.Timestamp,
	)
}

/*
Read returns the metric entry for the given key.
In compliance with the WORM model, a Measurement can not be read
before it is finalized.
*/
func (measurement *Measurement) Read(keys ...string) iter.Seq[*MetricEntry] {
	if !measurement.locked() {
		return func(yield func(*MetricEntry) bool) {
			yield(&MetricEntry{
				Err: errnie.Error(errnie.Err(
					errnie.Forbidden,
					"[data.measurement] not finalized",
					nil,
				)),
			})
		}
	}

	return func(yield func(*MetricEntry) bool) {
		for _, entry := range measurement.metrics {
			if entry != nil && entry.Metric != nil && (len(keys) == 0 || slices.Contains(keys, entry.Metric.label)) {
				if !yield(entry) {
					return
				}
			}
		}
	}
}

/*
Write the metrics to the Measurement and finalize it.
This means that the Measurement is now locked and cannot be changed.
It is therefore exactly "write once".
*/
func (measurement *Measurement) Write(
	metrics ...*Metric,
) *Measurement {
	if measurement.locked() {
		errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] measurement locked",
			nil,
		))

		return measurement
	}

	for _, metric := range metrics {
		found := measurement.entry(metric.label)

		if found != nil {
			prior := found.Metric.raw
			found.Metric.raw = metric.raw
			found.Metric.exact = metric.exact
			found.Metric.unit = metric.unit
			found.Metric.timescale = metric.timescale

			if metric.snm != 0 {
				found.Metric.snm = metric.snm
				continue
			}

			found.Metric.setSNM(prior)
			continue
		}

		measurement.metrics = append(measurement.metrics, &MetricEntry{
			Key:    metric.label,
			Metric: metric,
		})
	}

	measurement.finalize()
	return measurement
}

/*
ApproximateBytes returns an estimate of the serialized byte size of the Measurement.
*/
func (measurement *Measurement) ApproximateBytes() int64 {
	if !measurement.locked() {
		err := errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		))

		measurement.err = errors.Join(measurement.err, err)
		return 0
	}

	size := int64(128 + len(measurement.Label) + len(measurement.Source))

	for _, entry := range measurement.metrics {
		if entry != nil {
			size += int64(64 + len(entry.Key))
		}
	}

	for _, entry := range measurement.metadata {
		if entry != nil {
			size += int64(32 + len(entry.Key) + len(entry.Value))
		}
	}

	return size
}

/*
Meta returns an iterator over metadata entries matching keys.
If no keys are given, all metadata entries are returned.
*/
func (measurement *Measurement) Meta(keys ...string) iter.Seq[*StringEntry] {
	if !measurement.locked() {
		return func(yield func(*StringEntry) bool) {
			err := errnie.Error(errnie.Err(
				errnie.Forbidden,
				"[data.measurement] not finalized",
				nil,
			))

			measurement.err = errors.Join(measurement.err, err)

			yield(&StringEntry{
				Err: err,
			})
		}
	}

	return func(yield func(*StringEntry) bool) {
		for _, entry := range measurement.metadata {
			if entry != nil && (len(keys) == 0 || slices.Contains(keys, entry.Key)) {
				if !yield(entry) {
					return
				}
			}
		}
	}
}

/*
SetMeta sets the metadata of the Measurement.
It does nothing if the Measurement is locked.
*/
func (measurement *Measurement) SetMeta(key string, value string) {
	if measurement.locked() {
		errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] measurement locked",
			nil,
		))

		return
	}

	measurement.metadata = append(measurement.metadata, &StringEntry{
		Key:   key,
		Value: value,
	})
}

/*
Peers returns the peers of the Measurement.
*/
func (measurement *Measurement) Peers(peers ...*Measurement) []*Measurement {
	if len(peers) == 0 {
		if !measurement.locked() {
			err := errnie.Error(errnie.Err(
				errnie.Forbidden,
				"[data.measurement] not finalized",
				nil,
			))

			measurement.err = errors.Join(measurement.err, err)
			return nil
		}

		return measurement.peers
	}

	if measurement.locked() {
		errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] measurement locked",
			nil,
		))

		errnie.Error(measurement.err)
		return measurement.peers
	}

	for _, peer := range peers {
		if peer != nil {
			measurement.peers = append(measurement.peers, peer)
		}
	}

	return measurement.peers
}

/*
/*
Finalize locks the Measurement and validates it.
*/
func (measurement *Measurement) Finalize() *Measurement {
	if measurement.locked() {
		return measurement
	}

	return measurement.finalize()
}

func (measurement *Measurement) finalize() *Measurement {
	if measurement.locked() {
		errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] measurement locked",
			nil,
		))

		return measurement
	}

	if measurement.samples <= 0 {
		measurement.samples = 1
	}

	measurement.energy = 0

	for _, entry := range measurement.metrics {
		if entry != nil && entry.Metric != nil {
			measurement.err = errors.Join(measurement.err, entry.Metric.finalize())
			measurement.energy += entry.Metric.snm * entry.Metric.snm
		}
	}

	for _, peer := range measurement.peers {
		if peer != nil {
			measurement.energy += peer.energy
		}
	}

	if measurement.Timestamp <= 0 {
		measurement.Timestamp = time.Now().UnixNano()
	}
	measurement.setMaturity()
	measurement.ID = uuid.New().ID()

	// Fully finalize the Measurement by writing its ID.
	measurement.valid()
	return measurement
}

/*
Surprise returns the MDL prediction error: how much the current
total metric energy deviates from the expected regime baseline.
*/
func (measurement *Measurement) Surprise() float64 {
	if !measurement.locked() {
		err := errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		))

		measurement.err = errors.Join(measurement.err, err)
		return 0
	}

	// Zero expected energy means any energy is complete surprise
	if measurement.prediction <= 0 {
		return measurement.energy
	}

	return math.Abs(measurement.energy - measurement.prediction)
}

/*
Maturity returns sensor readiness [0, 1] based on empirical sample history
and completeness. Enforces locked WORM state.
*/
func (measurement *Measurement) Maturity() float64 {
	if !measurement.locked() {
		err := errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		))

		measurement.err = errors.Join(measurement.err, err)
		return 0
	}

	return measurement.maturity
}

/*
Samples returns the cumulative count of observations accumulated into this
measurement stream. Enforces locked WORM state.
*/
func (measurement *Measurement) Samples() int64 {
	if !measurement.locked() {
		err := errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		))

		measurement.err = errors.Join(measurement.err, err)
		return 0
	}

	return measurement.samples
}

/*
Energy returns total squared SNM metric excitation across the measurement.
Enforces locked WORM state.
*/
func (measurement *Measurement) Energy() float64 {
	if !measurement.locked() {
		err := errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		))

		measurement.err = errors.Join(measurement.err, err)
		return 0
	}

	return measurement.energy
}

/*
Prediction returns expected energy based on empirical EMA history.
Enforces locked WORM state.
*/
func (measurement *Measurement) Prediction() float64 {
	if !measurement.locked() {
		err := errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		))

		measurement.err = errors.Join(measurement.err, err)
		return 0
	}

	return measurement.prediction
}

/*
setMaturity calculates sensor readiness based strictly on
empirical sample history and metric completeness (warm-up gate).
*/
func (measurement *Measurement) setMaturity() *Measurement {
	if measurement.locked() {
		errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] measurement locked",
			nil,
		))

		return measurement
	}

	if measurement.samples <= 0 {
		measurement.maturity = 0
		return measurement
	}

	samplesCount := float64(measurement.samples)

	// Warm-up curve: approaches 1.0 as samples accumulate
	// (At 10 samples: ~0.90, at 50 samples: ~0.98, at 500: ~0.998)
	measurement.maturity = 1.0 - (1.0 / samplesCount)

	// Completeness penalty: if expected metrics are missing, scale down
	if expected := float64(len(SignalMetrics[measurement.Source])); expected > 0 {
		actual := float64(len(measurement.metrics))
		if actual < expected {
			measurement.maturity *= (actual / expected)
		}
	}

	return measurement
}

/*
Error returns the Measurement error.
*/
func (measurement *Measurement) Error() error {
	return measurement.err
}

/*
locked protects the WORM guarantee of the Measurement.
Once finalized, a Measurement cannot be changed.
*/
func (measurement *Measurement) locked() bool {
	return measurement != nil && measurement.ID != 0
}

/*
valid checks if the Measurement is valid.
*/
func (measurement *Measurement) valid() *Measurement {
	if measurement == nil {
		return nil
	}

	check := func(condition bool, message string) {
		if condition {
			measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
				errnie.Validation, message, nil,
			), "label", measurement.Label, "source", measurement.Source))
		}
	}

	check(measurement.ID == 0, "ID is required")
	check(measurement.Epoch < 0 || (measurement.Epoch == 0 && measurement.Source != "policy"), "epoch is required")
	check(measurement.Label == "", "label is required")
	check(measurement.Source == "", "source is required")
	check(measurement.At.IsZero(), "at is required")
	check(measurement.Tick < 0, "tick must be non-negative")
	check(measurement.SeqIdx < 0, "seqIdx must be non-negative")

	if measurement.err != nil {
		return nil
	}

	return measurement
}

/*
Raw returns the raw wire payload stored under key "raw" in metadata, if present.
*/
func (measurement *Measurement) Raw() []byte {
	if measurement == nil {
		return nil
	}

	for _, entry := range measurement.metadata {
		if entry != nil && entry.Key == "raw" {
			return []byte(entry.Value)
		}
	}

	return nil
}

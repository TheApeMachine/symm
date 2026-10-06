package data

import (
	"errors"
	"iter"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
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
	coherence  float64        // Cross-metric coherence: Jain's fairness index of |z_i| in [0, 1].
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
	seqIdx int64,
	tick int64,
	metadata ...*StringEntry,
) *Measurement {
	return &Measurement{
		Epoch:    epoch,
		Label:    label,
		Source:   source,
		SeqIdx:   seqIdx,
		Tick:     tick,
		metrics:  make([]*MetricEntry, 0),
		metadata: metadata,
		peers:    make([]*Measurement, 0),
	}
}

/*
Next instantiates a new Measurement, using the current instance
as its prior, copying over the center and scale of all metrics.
*/
func (measurement *Measurement) Next(source string, values map[string]float64) *Measurement {
	next := NewMeasurement(
		measurement.Epoch,
		measurement.Label,
		source,
		system.SeqIdx.Add(1),
		system.Tick.Load(),
		measurement.metadata...,
	)

	next.At = measurement.At
	next.From = measurement.From
	next.Timestamp = time.Now().UnixNano()
	next.samples = measurement.samples
	next.prediction = measurement.prediction
	next.peers = append(next.peers, measurement.peers...)

	for entry := range measurement.Read() {
		out := *entry.Metric
		out.Raw = values[entry.Key]
		out.Exact = nil
		out.Normalized = 0
		out.Standardized = 0

		next.metrics = append(next.metrics, &MetricEntry{
			Key:    entry.Key,
			Metric: &out,
		})
	}

	return next
}

/*
Read returns the metric entry for the given key.
In compliance with the WORM model, a Measurement can not be read
before it is finalized.
*/
func (measurement *Measurement) Read(keys ...string) iter.Seq[*MetricEntry] {
	if !measurement.locked() {
		return func(yield func(*MetricEntry) bool) {
			err := errnie.Error(errnie.Err(
				errnie.Forbidden,
				"[data.measurement] not finalized",
				nil,
			))
			measurement.err = errors.Join(measurement.err, err)

			yield(&MetricEntry{
				Err: err,
			})
		}
	}

	if len(keys) > 0 {
		return func(yield func(*MetricEntry) bool) {
			for _, key := range keys {
				for _, metricEntry := range measurement.metrics {
					if metricEntry.Metric.Label == key {
						if !yield(metricEntry) {
							return
						}
					}
				}
			}
		}
	}

	return func(yield func(*MetricEntry) bool) {
		for _, metricEntry := range measurement.metrics {
			if !yield(metricEntry) {
				return
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
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] locked",
			nil,
		)))

		return measurement
	}

	for _, metric := range metrics {
		var found *MetricEntry
		for _, entry := range measurement.metrics {
			if entry != nil && entry.Key == metric.Label {
				found = entry
				break
			}
		}

		if found != nil {
			found.Metric.Raw = metric.Raw
			found.Metric.Exact = metric.Exact
			found.Metric.unit = metric.unit
			found.Metric.timescale = metric.timescale
			continue
		}

		measurement.metrics = append(measurement.metrics, &MetricEntry{
			Key:    metric.Label,
			Metric: metric,
		})
	}

	measurement.finalize()
	return measurement
}

/*
Meta returns the metadata for a key.
*/
func (measurement *Measurement) Meta(key string) string {
	if !measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		)))

		return ""
	}

	for _, entry := range measurement.metadata {
		if entry == nil {
			continue
		}

		if entry.Key == key {
			return entry.Value
		}
	}

	return ""
}

/*
Peers returns the peers of the Measurement.
*/
func (measurement *Measurement) Peers(peers ...*Measurement) []*Measurement {
	if len(peers) == 0 && !measurement.locked() {
		measurement.err = errors.Join(errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		)))

		return nil
	}

	if len(peers) > 0 {
		measurement.peers = append(measurement.peers, peers...)
	}

	return measurement.peers
}

/*
finalize the Measurement, which locks the Measurement and validates it.
*/
func (measurement *Measurement) finalize() *Measurement {
	if measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] locked",
			nil,
		)))

		return measurement
	}

	// Count this observation before any Welford update, so metric and
	// Measurement statistics agree on n and never divide by zero.
	measurement.samples++

	for _, metric := range measurement.metrics {
		measurement.err = errors.Join(
			measurement.err,
			metric.Metric.finalize(float64(measurement.samples)),
		)
	}

	measurement.Timestamp = time.Now().UnixNano()
	measurement.setCoherence()
	measurement.setMaturity()

	// Fully finalize the Measurement by writing its ID.
	return measurement.valid()
}

/*
Coherence returns the cross-metric coherence (Jain's fairness index of
standardized magnitudes) for the Measurement.
*/
func (measurement *Measurement) Coherence() float64 {
	if !measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		)))

		return 0
	}

	return measurement.coherence
}

/*
Maturity returns the maturity for the Measurement.
*/
func (measurement *Measurement) Maturity() float64 {
	if !measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		)))

		return 0
	}

	return measurement.maturity
}

/*
Confidence is temporal maturity times cross-metric coherence:
how unsurprising the observation's energy is relative to the regime,
scaled by how evenly that energy is distributed across its metrics.
Both factors lie in [0, 1].
*/
func (measurement *Measurement) Confidence() float64 {
	if !measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		)))

		return 0
	}

	return measurement.maturity * measurement.coherence
}

/*
Error returns the Measurement error.
*/
func (measurement *Measurement) Error() error {
	return measurement.err
}

/*
setCoherence calculates cross-metric coherence using Jain's fairness index
on the absolute standardized metric magnitudes:

	coherence = (\sum |z_i|)^2 / (N * \sum z_i^2)

It measures how evenly the Measurement's standardized activity is distributed
across its Metrics. If all Metrics participate equally, coherence is 1.0. If
only one Metric carries the observation, coherence is 1/N. If all Metrics are
quiescent (zero energy), coherence is 0.
*/
func (measurement *Measurement) setCoherence() *Measurement {
	if measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] locked",
			nil,
		)))

		return measurement
	}

	count := float64(len(measurement.metrics))

	if count == 0 {
		measurement.coherence = 0
		return measurement
	}

	var sumAbs float64
	measurement.energy = 0

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil {
			continue
		}

		z := entry.Metric.Standardized
		sumAbs += math.Abs(z)
		measurement.energy += z * z
	}

	if count > 0 && measurement.energy > 0 {
		measurement.coherence = (sumAbs * sumAbs) / (count * measurement.energy)
	} else {
		measurement.coherence = 0
	}

	return measurement
}

/*
setMaturity calculates temporal maturity directly against the prior prediction
of total standardized metric energy using online Minimum Description Length (MDL).
Runs in O(1) time using the energy computed by setCoherence.
*/
func (measurement *Measurement) setMaturity() *Measurement {
	if measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] locked",
			nil,
		)))

		return measurement
	}

	n := float64(measurement.samples)

	// 1. Calculate maturity against PRIOR prediction (surprise)
	measurement.maturity = (core.Unit - core.Unit/n) / (core.Unit + math.Abs(
		measurement.energy-measurement.prediction,
	))

	// 2. Update prediction for next time
	measurement.prediction += (core.Unit / n) * (measurement.energy - measurement.prediction)

	return measurement
}

/*
locked protects the WORM guarantee of the Measurement.
Once finalized, a Measurement cannot be changed.
*/
func (measurement *Measurement) locked() bool {
	return measurement.ID != 0
}

/*
valid checks if the Measurement is valid.
*/
func (measurement *Measurement) valid() *Measurement {
	if measurement.locked() {
		return measurement
	}

	for _, metricEntry := range measurement.metrics {
		if err := metricEntry.Metric.valid(); err != nil {
			measurement.err = errors.Join(measurement.err, err)
		}
	}

	// metrics, metadata and peers are optional unless requested explicitly
	// (valid("metrics"), ...). A runtime join carries only Peers, and an
	// arena-allocated Measurement starts with a nil metrics slice.
	measurement.ID = uuid.New().ID()

	if err := errnie.Error(errnie.Require(map[string]any{
		"ID":        measurement.ID,
		"epoch":     measurement.Epoch,
		"label":     measurement.Label,
		"source":    measurement.Source,
		"seqIdx":    measurement.SeqIdx,
		"tick":      measurement.Tick,
		"timestamp": measurement.Timestamp,
		"at":        measurement.At,
		"from":      measurement.From,
		"maturity":  measurement.maturity,
		"coherence": measurement.coherence,
	})); err != nil {
		measurement.err = errors.Join(measurement.err, err)
	}

	return measurement
}

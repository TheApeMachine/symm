package data

import (
	"errors"
	"iter"
	"math"
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
NewFromPrior is used when we want to create a new Measurement from the incoming
prior, when we do not have a stored Measurement for the source and label yet.
*/
func NewFromPrior(
	prior *Measurement,
	source string,
	values map[string]float64,
	peers ...*Measurement,
) *Measurement {
	meas := NewMeasurement(
		prior.Epoch,
		prior.Label,
		source,
		system.SeqIdx.Add(1),
		prior.Tick,
		prior.Metadata()...,
	)

	meas.At = prior.At
	meas.From = prior.From

	if len(peers) > 0 {
		meas.Peers(peers...)
	}

	meas.copyMetrics(source, values, prior)
	meas.finalize()
	return meas
}

/*
Next instantiates the source's Measurement derived from the current instance
for the event prior. It carries over identity, metadata, and peers, takes the
event's venue time and ingress tick, and holds exactly the values the source
produced: the prior's own metrics belong to the prior.
*/
func (measurement *Measurement) Next(values map[string]float64) *Measurement {
	// Each successor owns copies of the metadata entries, so SetMeta on one
	// never rewrites its prior or a sibling built from the same prior.
	metadata := make([]*StringEntry, 0, len(measurement.metadata)+1)

	for _, entry := range measurement.metadata {
		if entry != nil {
			owned := *entry
			metadata = append(metadata, &owned)
		}
	}

	next := NewMeasurement(
		measurement.Epoch,
		measurement.Label,
		measurement.Source,
		system.SeqIdx.Add(1),
		system.Tick.Load(),
		metadata...,
	)

	next.From = measurement.At
	next.Timestamp = time.Now().UnixNano()
	next.At = time.Unix(0, next.Timestamp).UTC()
	next.peers = append(next.peers, measurement.peers...)

	next.samples = measurement.samples + 1
	next.prediction = measurement.prediction

	next.copyMetrics(measurement.Source, values, measurement)

	next.finalize()
	return next
}

func (measurement *Measurement) copyMetrics(
	source string, values map[string]float64, prior *Measurement,
) {
	if len(values) > 0 {
		for key, val := range values {
			unit, timescale := GetDimension(source, key)
			metric := NewMetric(key, val, unit, timescale)

			if prior != nil {
				for _, entry := range prior.metrics {
					if entry != nil && entry.Key == key && entry.Metric != nil {
						metric.setSNM(entry.Metric.raw)
						break
					}
				}
			}

			measurement.metrics = append(measurement.metrics, &MetricEntry{
				Key:    key,
				Metric: metric,
			})
		}
	}
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
					if metricEntry.Metric.label == key {
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
			if entry != nil && entry.Key == metric.label {
				found = entry
				break
			}
		}

		if found != nil {
			found.Metric.raw = metric.raw
			found.Metric.exact = metric.exact
			found.Metric.unit = metric.unit
			found.Metric.timescale = metric.timescale
			continue
		}

		measurement.metrics = append(measurement.metrics, &MetricEntry{
			Key:    metric.label,
			Metric: metric,
		})
	}

	measurement.finalize()
	return measurement.valid()
}

/*
ApproximateBytes returns an estimate of the serialized byte size of the Measurement.
*/
func (measurement *Measurement) ApproximateBytes() int64 {
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
Metadata returns the metadata entries of the Measurement.
*/
func (measurement *Measurement) Metadata() []*StringEntry {
	if !measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] not finalized",
			nil,
		)))

		return nil
	}

	return measurement.metadata
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
SetError joins an error to the measurement error.
*/
func (measurement *Measurement) SetError(err error) {
	if err != nil {
		measurement.err = errors.Join(measurement.err, err)
	}
}

/*
SetMeta sets or updates a metadata entry on the measurement.
*/
func (measurement *Measurement) SetMeta(key string, value string) {
	for _, entry := range measurement.metadata {
		if entry != nil && entry.Key == key {
			entry.Value = value
			return
		}
	}

	measurement.metadata = append(measurement.metadata, &StringEntry{
		Key:   key,
		Value: value,
	})
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

	if measurement.samples <= 0 {
		measurement.samples = 1
	}

	var sumAbs float64
	measurement.energy = 0

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil {
			continue
		}

		measurement.err = errors.Join(
			measurement.err,
			entry.Metric.finalize(),
		)

		snm := entry.Metric.snm
		sumAbs += math.Abs(snm)
		measurement.energy += snm * snm
	}

	metricCount := float64(len(measurement.metrics))
	measurement.coherence = 0

	if metricCount > 0 && measurement.energy > 0 {
		measurement.coherence = (sumAbs * sumAbs) / (metricCount * measurement.energy)
	}

	for _, peer := range measurement.peers {
		if peer != nil {
			measurement.energy += peer.energy
		}
	}

	measurement.Timestamp = time.Now().UnixNano()
	measurement.setMaturity()

	// Fully finalize the Measurement by writing its ID.
	return measurement.valid()
}

/*
Surprise returns the MDL prediction error: how much the current
total metric energy deviates from the expected regime baseline.
*/
func (measurement *Measurement) Surprise() float64 {
	if !measurement.locked() {
		return 0
	}

	// Zero expected energy means any energy is complete surprise
	if measurement.prediction <= 0 {
		return measurement.energy
	}

	return math.Abs(measurement.energy - measurement.prediction)
}

/*
Confidence returns temporal maturity times cross-metric coherence in [0, 1].
*/
func (measurement *Measurement) Confidence() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.maturity * measurement.coherence
}

/*
Coherence returns cross-metric coherence in [0, 1].
*/
func (measurement *Measurement) Coherence() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.coherence
}

/*
Maturity returns temporal maturity in [0, 1].
*/
func (measurement *Measurement) Maturity() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.maturity
}

/*
Energy returns total metric excitation energy (sum of squared SNM).
*/
func (measurement *Measurement) Energy() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.energy
}

/*
Prediction returns the online expected baseline energy.
*/
func (measurement *Measurement) Prediction() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.prediction
}

/*
Samples returns the cumulative count of observations seen by this stream.
*/
func (measurement *Measurement) Samples() int64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.samples
}

/*
setMaturity calculates sensor readiness based strictly on
empirical sample history and metric completeness (warm-up gate).
*/
func (measurement *Measurement) setMaturity() *Measurement {
	if measurement.locked() {
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

	// Update the online prediction of energy for the NEXT tick
	measurement.prediction += (1.0 / samplesCount) * (measurement.energy - measurement.prediction)

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
	return measurement.ID != 0
}

/*
valid checks if the Measurement is valid.
*/
func (measurement *Measurement) valid() *Measurement {
	for _, metricEntry := range measurement.metrics {
		if metricEntry != nil && metricEntry.Metric != nil {
			if err := metricEntry.Metric.valid(); err != nil {
				measurement.err = errors.Join(measurement.err, err)
			}
		}
	}

	for _, peer := range measurement.peers {
		if peer != nil && peer.valid() != nil {
			measurement.err = errors.Join(measurement.err, peer.Error())
		}
	}

	if measurement.ID == 0 {
		measurement.ID = uuid.New().ID()
	}

	if measurement.Epoch <= 0 {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Validation, "epoch is required", nil,
		), "label", measurement.Label, "source", measurement.Source))
	}

	if measurement.Label == "" {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Validation, "label is required", nil,
		), "label", measurement.Label, "source", measurement.Source))
	}

	if measurement.Source == "" {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Validation, "source is required", nil,
		), "label", measurement.Label, "source", measurement.Source))
	}

	if measurement.At.IsZero() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Validation, "at is required", nil,
		), "label", measurement.Label, "source", measurement.Source))
	}

	if measurement.Tick < 0 {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Validation, "tick must be non-negative", nil,
		), "label", measurement.Label, "source", measurement.Source))
	}

	if measurement.SeqIdx < 0 {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Validation, "seqIdx must be non-negative", nil,
		), "label", measurement.Label, "source", measurement.Source))
	}

	return measurement
}

package data

import (
	"errors"
	"iter"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
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
	snr        float64        // Signal-to-noise ratio (Statistical Mean-to-Standard Deviation).
	maturity   float64        // Maturity of the Measurement.
	samples    int64          // Observations accumulated in the current regime.
	energy     float64        // Total metric excitation energy (sum of z^2).
	prediction float64        // Expected energy baseline from prior.
	err        error          // Error if any.
	metrics    []*MetricEntry // Metrics owned by this Measurement.
	metadata   []*StringEntry // Metadata owned by this Measurement.
	peers      []*Measurement // Peers, used to group related Measurements.
	owner      *ArenaOwner    // Producer arena; enables prior capture on finalize.
	prior      *priorSnapshot // Prior pinned at alloc; seeds metric Welford state.
	resetEpoch uint64         // ArenaOwner reset epoch at alloc; gates prior capture.
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
Read returns the metric entry for the given key.
In compliance with the WORM model, a Measurement can not be read
before it is finalized.
*/
func (measurement *Measurement) Read(keys ...string) iter.Seq[*MetricEntry] {
	if !measurement.locked() {
		return func(yield func(*MetricEntry) bool) {
			// Forbidden is returned, not logged: callers probing peers that
			// are still being written must see the error without a log flood.
			if !yield(&MetricEntry{
				Err: errors.Join(measurement.err, errnie.Err(
					errnie.Forbidden,
					"[data.measurement] not finalized",
					nil,
				)),
			}) {
				return
			}
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
	for _, entry := range measurement.metadata {
		if entry.Key == key {
			return entry.Value
		}
	}

	return ""
}

/*
Peers returns the peers of the Measurement.
*/
func (measurement *Measurement) Peers() []*Measurement {
	if !measurement.locked() {
		return nil
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

	// Seed metric Welford state from the prior pinned at alloc (same
	// snapshot samples/prediction came from) before this observation
	// updates center/scale.
	if measurement.prior != nil {
		for _, entry := range measurement.metrics {
			if entry == nil || entry.Metric == nil {
				continue
			}
			if seed, ok := measurement.prior.metric(entry.Metric.Label); ok {
				entry.Metric.center = seed.center
				entry.Metric.scale = seed.scale
			}
		}
	}

	for _, metric := range measurement.metrics {
		measurement.err = errors.Join(
			measurement.err,
			metric.Metric.finalize(float64(measurement.samples)),
		)
	}

	measurement.Timestamp = time.Now().UnixNano()
	measurement.setSNR()
	measurement.setMaturity()

	// Fully finalize the Measurement by writing its ID.
	measurement.ID = uuid.New().ID()
	measurement.valid()

	// Hold transferable stats on the ArenaOwner until the next
	// NewMeasurement for this Label copies them, replacing this snapshot.
	if measurement.owner != nil {
		measurement.owner.capturePrior(measurement)
	}

	return measurement
}

/*
SNR returns the signal-to-noise ratio for the Measurement.
*/
func (measurement *Measurement) SNR() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.snr
}

/*
Maturity returns the maturity for the Measurement.
*/
func (measurement *Measurement) Maturity() float64 {
	if !measurement.locked() {
		return 0
	}

	return measurement.maturity
}

/*
Error returns the Measurement error.
*/
func (measurement *Measurement) Error() error {
	return measurement.err
}

/*
setSNR calculates the Signal-to-Noise ratio for the Measurement.
Each Metric is a stand-alone value, but the Metrics within a Measurement are not
truly independent. They observe different expressions of the same underlying
phenomenon and are therefore subject to the same governing forces. This creates
an implicit coupling between them: degradation or loss of confidence in one
Metric is evidence that the shared observation itself may be deteriorating, and
should therefore reduce confidence in the other Metrics within that Measurement as well.
*/
func (measurement *Measurement) setSNR() *Measurement {
	if measurement.locked() {
		measurement.err = errors.Join(measurement.err, errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[data.measurement] locked",
			nil,
		)))
	}

	var mean, m2, count float64

	for _, entry := range measurement.metrics {
		x := math.Abs(entry.Metric.Standardized)
		count++
		delta := x - mean
		mean += delta / count
		m2 += delta * (x - mean)
	}

	if count > 0 && m2 > 0 {
		measurement.snr = mean / math.Sqrt(m2/count)
	} else {
		measurement.snr = mean
	}

	return measurement.valid("snr")
}

/*
setMaturityAndSNR calculates both SNR and Maturity directly from the total
standardized metric energy (sum of z^2) using online Minimum Description Length (MDL).
Runs in O(metrics) time with zero heap allocations.
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

	measurement.energy = 0

	for _, entry := range measurement.metrics {
		measurement.energy += entry.Metric.Standardized * entry.Metric.Standardized
	}

	n := float64(measurement.samples)

	// 1. Calculate maturity against PRIOR prediction (surprise)
	measurement.maturity = math.Max(
		0.0, math.Min(
			core.Unit,
			(core.Unit-core.Unit/n)*(core.Unit/(core.Unit+math.Abs(
				measurement.energy-measurement.prediction,
			))),
		),
	)

	// 2. Update prediction for next time
	measurement.prediction += (core.Unit / n) * (measurement.energy - measurement.prediction)

	return measurement.valid(
		"maturity", "energy", "samples", "prediction",
	)
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
func (measurement *Measurement) valid(fields ...string) *Measurement {
	if len(fields) > 0 {
		mapped := make(map[string]any)

		for _, field := range fields {
			switch field {
			case "ID":
				mapped[field] = measurement.ID
			case "epoch":
				mapped[field] = measurement.Epoch
			case "label":
				mapped[field] = measurement.Label
			case "source":
				mapped[field] = measurement.Source
			case "seqIdx":
				mapped[field] = measurement.SeqIdx
			case "tick":
				mapped[field] = measurement.Tick
			case "timestamp":
				mapped[field] = measurement.Timestamp
			case "at":
				mapped[field] = measurement.At
			case "from":
				mapped[field] = measurement.From
			case "maturity":
				mapped[field] = measurement.maturity
			case "snr":
				mapped[field] = measurement.snr
			case "err":
				mapped[field] = measurement.err
			case "metrics":
				mapped[field] = measurement.metrics
			case "metadata":
				mapped[field] = measurement.metadata
			case "peers":
				mapped[field] = measurement.peers
			}
		}

		measurement.err = errors.Join(
			measurement.err,
			errnie.Error(errnie.Require(mapped)),
		)

		return measurement
	}

	for _, metricEntry := range measurement.metrics {
		if err := metricEntry.Metric.valid(); err != nil {
			measurement.err = errors.Join(measurement.err, err)
		}
	}

	// metadata and peers are optional unless valid("metadata") /
	// valid("peers") is requested explicitly.
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
		"snr":       measurement.snr,
		"metrics":   measurement.metrics,
	})); err != nil {
		measurement.err = errors.Join(measurement.err, err)
	}

	return measurement
}

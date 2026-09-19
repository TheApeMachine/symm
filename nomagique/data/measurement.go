package data

import (
	"maps"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Measurement is the native data type in nomagique, and in most cases should be
leveraged to build a system that is easy to work with, because of a mostly
mono-typed architecture.

It implements Identifiable, so it can work with nomagique stores that use index
addressable storage slots for O(1) reads. The register yields a working copy of
a slot; published snapshots that appear as peers are not written.

Label is often used as a named canonical group that makes sense in a given
project, while SeqIdx is the workspace observation index used as a
synchronization anchor. Zero means the observation has not been stamped.

At should generally always be set to the timestamp of the event that fills
the Measurement, while From is optional, but has value beyond just defining
a window of time for the measurement. It can also be used to derive rudimentary
performance and latency diagnostics.

Quality is not caller-supplied. Maturity and SNR are derived by the Finalizer
primitive from the measurement's own estimator facts, these values represent
the amount of trust to put in the overal Measurement, and the Metrics it contains.
*/
type Measurement[T any] struct {
	ID         T                    `json:"id"`
	Label      string               `json:"label"`
	Source     string               `json:"source"`
	SeqIdx     int64                `json:"seqIdx"`
	Timestamp  int64                `json:"timestamp"`
	At         time.Time            `json:"at"`
	From       time.Time            `json:"from,omitempty"`
	Maturity   float64              `json:"maturity"`
	SNR        float64              `json:"snr"`
	SNRDefined bool                 `json:"snrDefined"`
	Estimated  bool                 `json:"estimated"`
	Err        error                `json:"-"`
	Metrics    map[string]Metric[T] `json:"metrics,omitempty"`
	Metadata   map[string]string    `json:"metadata,omitempty"`
	Provenance map[string]string    `json:"provenance,omitempty"`
	Peers      []*Measurement[T]    `json:"peers"`
	// Result is the completed, immutable structured output of this observation.
	// The register and tees share it; numeric persistence uses Metrics.
	Result any `json:"-"`
}

/*
NewMeasurement creates one identified observation with empty metric storage.
Its identity is unstamped: the register slot that owns it is assigned when a
workload registers it, and the node stamps the slot back via SetID.
*/
func NewMeasurement[T any](
	source string, metrics map[string]Metric[T],
) *Measurement[T] {
	if metrics == nil {
		metrics = make(map[string]Metric[T])
	}

	return &Measurement[T]{
		Source:     source,
		Metrics:    metrics,
		Metadata:   make(map[string]string),
		Provenance: make(map[string]string),
	}
}

/*
Identity names the register slot this measurement's consumer node owns.
*/
func (measurement *Measurement[T]) Identity() T {
	return measurement.ID
}

/*
Identify names the register slot this measurement's consumer node owns.
*/
func (measurement *Measurement[T]) Identify(id T) T {
	measurement.ID = id
	return measurement.ID
}

/*
FindPeer returns the first peer matching the given predicate, or nil if none match.
*/
func (measurement *Measurement[T]) FindPeer(predicate func(*Measurement[T]) bool) *Measurement[T] {
	if measurement == nil || predicate == nil {
		return nil
	}

	for _, peer := range measurement.Peers {
		if peer != nil && predicate(peer) {
			return peer
		}
	}

	return nil
}

/*
Clone returns an independent copy of the measurement and its mappings. Peer
pointers are copied, not cloned: the register attaches live published snapshots.
*/
func (measurement *Measurement[T]) Clone() *Measurement[T] {
	if measurement == nil {
		return nil
	}

	metrics := make(map[string]Metric[T], len(measurement.Metrics))
	maps.Copy(metrics, measurement.Metrics)

	var metadata map[string]string

	if measurement.Metadata != nil {
		metadata = make(map[string]string, len(measurement.Metadata))
		maps.Copy(metadata, measurement.Metadata)
	}

	var provenance map[string]string

	if measurement.Provenance != nil {
		provenance = make(map[string]string, len(measurement.Provenance))
		maps.Copy(provenance, measurement.Provenance)
	}

	var peers []*Measurement[T]

	if len(measurement.Peers) != 0 {
		peers = make([]*Measurement[T], len(measurement.Peers))
		copy(peers, measurement.Peers)
	}

	return &Measurement[T]{
		ID:         measurement.ID,
		Label:      measurement.Label,
		Source:     measurement.Source,
		SeqIdx:     measurement.SeqIdx,
		Timestamp:  measurement.Timestamp,
		At:         measurement.At,
		From:       measurement.From,
		Maturity:   measurement.Maturity,
		SNR:        measurement.SNR,
		SNRDefined: measurement.SNRDefined,
		Estimated:  measurement.Estimated,
		Err:        measurement.Err,
		Metrics:    metrics,
		Metadata:   metadata,
		Provenance: provenance,
		Peers:      peers,
		Result:     measurement.Result,
	}
}

/*
Pull copies event identity, provenance, and the named metrics from other onto
this measurement. The source is not mutated. Identity, source, and metadata
stay with this measurement.
*/
func (measurement *Measurement[T]) Pull(other *Measurement[T], keys ...string) {
	if measurement == nil || other == nil {
		return
	}

	measurement.Label = other.Label
	measurement.At = other.At
	measurement.From = other.From
	measurement.SeqIdx = other.SeqIdx
	measurement.Timestamp = other.Timestamp

	if other.Provenance != nil {
		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string, len(other.Provenance))
		}

		maps.Copy(measurement.Provenance, other.Provenance)
	}

	if len(keys) == 0 {
		return
	}

	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]Metric[T], len(keys))
	}

	for _, key := range keys {
		if metric, ok := other.Metrics[key]; ok {
			measurement.Metrics[key] = metric
		}
	}
}



/*
Reset zeroes every metric's values in place, so a pre-allocated measurement
can flow through again without being reallocated. The declared schema never
moves.
*/
func (measurement *Measurement[T]) Reset() {
	for key, metric := range measurement.Metrics {
		metric.Raw = zero[T]()
		metric.Normalized = nil
		metric.Standardized = nil
		measurement.Metrics[key] = metric
	}
}

/*
zero is the type's zero value, for clearing a metric's observation.
*/
func zero[T any]() T {
	var value T

	return value
}

const (
	MetadataSupport        = "support"
	MetadataMaturity       = "maturity"
	MetadataDivergence     = "divergence"
	MetadataNoiseVariance  = "noise_variance"
	MetadataMahalanobisSNR = "mahalanobis_snr"
)


/*
Finalize derives the measurement's quality facts from its own estimator
metadata, mutating the measurement in place.
*/
func (measurement *Measurement[Value]) Finalize() {
	finalizer := NewFinalizer[Value]()
	finalizer(measurement)
}

/*
NewFinalizer creates the measurement quality derivation Value closure.
No structs, pure Value closure.
*/
type Finalizer[Value any] types.Value[*Measurement[Value], *Measurement[Value]]
func NewFinalizer[Value any]() Finalizer[Value] {
	quality := NewQuality()

	return func(measurement *Measurement[Value]) *Measurement[Value] {
		if measurement != nil {
			reading := quality(factsFromMetadata(measurement.Metadata))

			measurement.Maturity = reading.Maturity
			measurement.SNR = reading.SNR
			measurement.SNRDefined = reading.SNRDefined
			measurement.Estimated = reading.Estimated
		}

		return measurement
	}
}

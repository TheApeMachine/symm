package data

import (
	"errors"
	"iter"
	"maps"
	"math"
	"sort"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
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
	ID         int                  `json:"id"`
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
	inputs     map[string]Metric[T]
	mu         sync.RWMutex
	Metadata   map[string]string `json:"metadata,omitempty"`
	Provenance map[string]string `json:"provenance,omitempty"`
	Peers      []*Measurement[T] `json:"peers"`
	// Result is the completed, immutable structured output of this observation.
	// The register and tees share it; numeric persistence uses Metrics.
	Result any `json:"-"`
}

// GetMetric safely retrieves a metric, checking producer-owned Metrics first then inherited inputs.
func (m *Measurement[T]) GetMetric(key string) Metric[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Metrics != nil {
		if val, ok := m.Metrics[key]; ok {
			return val
		}
	}
	if m.inputs != nil {
		if val, ok := m.inputs[key]; ok {
			return val
		}
	}
	return Metric[T]{}
}

// LookupMetric safely retrieves a metric and a boolean indicating if it was found.
func (m *Measurement[T]) LookupMetric(key string) (Metric[T], bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Metrics != nil {
		if val, ok := m.Metrics[key]; ok {
			return val, true
		}
	}
	if m.inputs != nil {
		if val, ok := m.inputs[key]; ok {
			return val, true
		}
	}
	return Metric[T]{}, false
}

// SetMetric safely sets a metric owned by this producer.
func (m *Measurement[T]) SetMetric(key string, val Metric[T]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metrics == nil {
		m.Metrics = make(map[string]Metric[T])
	}
	m.Metrics[key] = val
}

// WriteMetric safely writes a value to a metric and updates it in the owned Metrics map.
func (m *Measurement[T]) WriteMetric(key string, val T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metrics == nil {
		m.Metrics = make(map[string]Metric[T])
	}
	metric, ok := m.Metrics[key]
	if !ok && m.inputs != nil {
		metric = m.inputs[key]
	}
	m.Metrics[key] = metric.Write(val)
}

// WriteStandardized sets the raw value and marks the metric as standardized.
func (m *Measurement[T]) WriteStandardized(key string, val T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metrics == nil {
		m.Metrics = make(map[string]Metric[T])
	}
	metric, ok := m.Metrics[key]
	if !ok && m.inputs != nil {
		metric = m.inputs[key]
	}
	metric = metric.Write(val)
	v := val
	metric.Standardized = &v
	m.Metrics[key] = metric
}

// WriteNormalized sets the raw value and marks the metric as normalized.
func (m *Measurement[T]) WriteNormalized(key string, val T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metrics == nil {
		m.Metrics = make(map[string]Metric[T])
	}
	metric, ok := m.Metrics[key]
	if !ok && m.inputs != nil {
		metric = m.inputs[key]
	}
	metric = metric.Write(val)
	v := val
	metric.Normalized = &v
	m.Metrics[key] = metric
}

// RangeMetrics safely iterates over all visible metrics (owned and inherited inputs).
func (m *Measurement[T]) RangeMetrics(f func(key string, metric Metric[T]) bool) {
	m.mu.RLock()
	all := make(map[string]Metric[T], len(m.Metrics)+len(m.inputs))
	if m.inputs != nil {
		maps.Copy(all, m.inputs)
	}
	if m.Metrics != nil {
		maps.Copy(all, m.Metrics)
	}
	m.mu.RUnlock()

	for k, v := range all {
		if !f(k, v) {
			return
		}
	}
}

// MetricsSnapshot returns a shallow copy of owned Metrics under the read lock.
// It contains only facts this producer actually created or changed.
func (m *Measurement[T]) MetricsSnapshot() map[string]Metric[T] {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Metrics == nil {
		return nil
	}
	out := make(map[string]Metric[T], len(m.Metrics))
	maps.Copy(out, m.Metrics)
	return out
}

// ProvenanceSnapshot returns a copy of Provenance under the read lock.
func (m *Measurement[T]) ProvenanceSnapshot() map[string]string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Provenance == nil {
		return nil
	}
	return maps.Clone(m.Provenance)
}


// MetadataSnapshot returns a copy of Metadata under the read lock.
func (m *Measurement[T]) MetadataSnapshot() map[string]string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Metadata == nil {
		return nil
	}
	return maps.Clone(m.Metadata)
}

// GetSource returns Source under the read lock.
func (m *Measurement[T]) GetSource() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Source
}

// SetSource sets Source under the write lock.
func (m *Measurement[T]) SetSource(source string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Source = source
}

/*
SetQuality publishes Finalizer/interpreter quality fields under the write lock.
Bare field stores race when a shared workspace slot is still reachable.
*/
func (m *Measurement[T]) SetQuality(maturity, snr float64, snrDefined, estimated bool) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Maturity = maturity
	m.SNR = snr
	m.SNRDefined = snrDefined
	m.Estimated = estimated
}

/*
Fork returns a working copy for one concurrent stage consumer.
Inputs from the shared measurement are inherited in inputs (readable via GetMetric/LookupMetric),
while Metrics starts empty so the consumer only owns facts it actually creates or changes.
*/
func (measurement *Measurement[T]) Fork() *Measurement[T] {
	if measurement == nil {
		return nil
	}

	measurement.mu.RLock()
	defer measurement.mu.RUnlock()

	forkInputs := make(map[string]Metric[T], len(measurement.inputs)+len(measurement.Metrics))
	if len(measurement.inputs) > 0 {
		maps.Copy(forkInputs, measurement.inputs)
	}
	if len(measurement.Metrics) > 0 {
		maps.Copy(forkInputs, measurement.Metrics)
	}

	var metadata map[string]string
	if len(measurement.Metadata) > 0 {
		metadata = maps.Clone(measurement.Metadata)
	}

	var provenance map[string]string
	if len(measurement.Provenance) > 0 {
		provenance = maps.Clone(measurement.Provenance)
	}

	var peers []*Measurement[T]
	if len(measurement.Peers) > 0 {
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
		Metrics:    make(map[string]Metric[T]),
		inputs:     forkInputs,
		Metadata:   metadata,
		Provenance: provenance,
		Peers:      peers,
		Result:     measurement.Result,
	}
}

/*
PersistClone is a queue/off-ramp snapshot: maps copied, Peers and inputs stripped so
StoreTee/UITee cannot retain the disruptor peer forest across drain lag.
*/
func (measurement *Measurement[T]) PersistClone() *Measurement[T] {
	if measurement == nil {
		return nil
	}
	out := measurement.Clone()
	if out != nil {
		out.Peers = nil
		out.inputs = nil
		out.Result = nil
	}
	return out
}

/*
Contribute attaches one independently owned producer fragment as a Peer on the
shared observation. Concurrent stage nodes Fork → Step → Contribute.

Ownership rules (memory-critical):
  - replace any existing peer with the same Source (bounded peer set)
  - never dump producer metrics onto the shared Metrics map (that exploded
    Grid cellKey and Iceberg map columns with source-qualified duplicates)
  - quality merge stays order-invariant mins
  - Result stays on the owned peer
  - producer contribution contains only facts that producer actually created or changed
*/
func (measurement *Measurement[T]) Contribute(owned *Measurement[T]) {
	if measurement == nil || owned == nil || measurement == owned {
		return
	}

	owned.mu.RLock()
	source := owned.Source
	maturity := owned.Maturity
	snr := owned.SNR
	snrDefined := owned.SNRDefined
	estimated := owned.Estimated
	owned.mu.RUnlock()

	// Producer fragments must not retain the shared peer forest or input references.
	owned.mu.Lock()
	owned.Peers = nil
	owned.inputs = nil
	owned.mu.Unlock()

	measurement.mu.Lock()
	defer measurement.mu.Unlock()

	replaced := false
	if source != "" {
		for index, peer := range measurement.Peers {
			if peer != nil && peer.Source == source {
				measurement.Peers[index] = owned
				replaced = true
				break
			}
		}
	}
	if !replaced {
		measurement.Peers = append(measurement.Peers, owned)
	}

	sort.SliceStable(measurement.Peers, func(i, j int) bool {
		left, right := measurement.Peers[i], measurement.Peers[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.SeqIdx < right.SeqIdx
	})

	mergeQualityLocked(measurement, maturity, snr, snrDefined, estimated)
}

// mergeQualityLocked folds producer quality into the shared slot. Caller holds mu.
// Min maturity and min defined SNR are commutative — contribute order does not matter.
func mergeQualityLocked[T any](
	measurement *Measurement[T], maturity, snr float64, snrDefined, estimated bool,
) {
	if maturity == 0 && !snrDefined && !estimated {
		return
	}

	if measurement.Maturity == 0 && !measurement.SNRDefined && !measurement.Estimated {
		measurement.Maturity = maturity
		measurement.SNR = snr
		measurement.SNRDefined = snrDefined
		measurement.Estimated = estimated
		return
	}

	if maturity > 0 {
		if measurement.Maturity == 0 || maturity < measurement.Maturity {
			measurement.Maturity = maturity
		}
	}

	if snrDefined {
		if !measurement.SNRDefined {
			measurement.SNR = snr
			measurement.SNRDefined = true
		} else if snr < measurement.SNR {
			measurement.SNR = snr
		}
	}

	measurement.Estimated = measurement.Estimated || estimated
}

// EnsureMetadata safely initializes the metadata map if it is nil.
func (m *Measurement[T]) EnsureMetadata() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}
}

// GetMetadata safely retrieves a metadata value.
func (m *Measurement[T]) GetMetadata(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Metadata == nil {
		return "", false
	}
	val, ok := m.Metadata[key]
	return val, ok
}

// SetMetadata safely sets a metadata value.
func (m *Measurement[T]) SetMetadata(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}
	m.Metadata[key] = value
}

// DeleteMetadata safely deletes a metadata value.
func (m *Measurement[T]) DeleteMetadata(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Metadata != nil {
		delete(m.Metadata, key)
	}
}

// RangeMetadata safely iterates over metadata.
func (m *Measurement[T]) RangeMetadata(f func(key, value string) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Metadata == nil {
		return
	}
	for k, v := range m.Metadata {
		if !f(k, v) {
			break
		}
	}
}

func (m *Measurement[T]) EnsureProvenance() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Provenance == nil {
		m.Provenance = make(map[string]string)
	}
}

func (m *Measurement[T]) GetProvenance(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.Provenance == nil {
		return "", false
	}

	val, ok := m.Provenance[key]
	return val, ok
}

func (m *Measurement[T]) SetProvenance(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Provenance == nil {
		m.Provenance = make(map[string]string)
	}

	m.Provenance[key] = value
}

func (m *Measurement[T]) DeleteProvenance(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Provenance != nil {
		delete(m.Provenance, key)
	}
}

func (m *Measurement[T]) RangeProvenance(f func(key, value string) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for k, v := range m.Provenance {
		if !f(k, v) {
			break
		}
	}
}

// Facts extracts QualityFacts from a Metadata snapshot. Never hand the live
// map to factsFromMetadata — concurrent SetMetadata/DeleteMetadata would
// fatal on concurrent map read/write even under RLock if any writer bypasses
// mu, and snapshot keeps the parse off the critical section.
func (m *Measurement[T]) Facts() QualityFacts {
	if m == nil {
		return QualityFacts{}
	}

	m.mu.RLock()
	snap := maps.Clone(m.Metadata)
	m.mu.RUnlock()

	return factsFromMetadata(snap)
}

/*
NewMeasurement creates one identified observation with empty metric storage.
Its identity is unstamped: the register slot that owns it is assigned when a
workload registers it, and the node stamps the slot back via SetID.
*/

/*
StampInterval sets At and an optional From window start. From is only stored when
it is not after At — out-of-order venue clocks must not flood Category with
inverted intervals.
*/
func StampInterval[T any](measurement *Measurement[T], at, from time.Time) {
	if measurement == nil {
		return
	}
	if !at.IsZero() {
		measurement.At = at
	}
	if from.IsZero() || (!measurement.At.IsZero() && from.After(measurement.At)) {
		return
	}
	measurement.From = from
}

func NewMeasurement[T any](
	source string, metrics map[string]Metric[T],
) *Measurement[T] {
	if metrics == nil {
		metrics = make(map[string]Metric[T])
	}

	return &Measurement[T]{
		ID:         -1,
		Source:     source,
		Metrics:    metrics,
		Metadata:   make(map[string]string),
		Provenance: make(map[string]string),
	}
}

// SetSeqIdx implements runtime.Sequencer for the measurement.
func (measurement *Measurement[T]) SetSeqIdx(seq int64) {
	if measurement != nil {
		measurement.SeqIdx = seq
	}
}

/*
Identity names the register slot this measurement's consumer node owns.
*/
func (measurement *Measurement[T]) Identity() int {
	return measurement.ID
}

/*
Identify names the register slot this measurement's consumer node owns.
*/
func (measurement *Measurement[T]) Identify(id int) Identifiable[T] {
	measurement.ID = id
	return measurement
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

	measurement.mu.RLock()
	defer measurement.mu.RUnlock()

	var metrics map[string]Metric[T]
	if len(measurement.Metrics) > 0 {
		metrics = make(map[string]Metric[T], len(measurement.Metrics))
		maps.Copy(metrics, measurement.Metrics)
	}

	var inputs map[string]Metric[T]
	if len(measurement.inputs) > 0 {
		inputs = make(map[string]Metric[T], len(measurement.inputs))
		maps.Copy(inputs, measurement.inputs)
	}

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
		inputs:     inputs,
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

	// Snapshot other under its read lock, then apply under this measurement's
	// write lock. Never touch Provenance/Metrics maps without mu — disruptor
	// HandlerGroups share one slot across concurrent consumers.
	other.mu.RLock()
	label := other.Label
	at := other.At
	from := other.From
	seq := other.SeqIdx
	timestamp := other.Timestamp
	var provenance map[string]string
	if len(other.Provenance) > 0 {
		provenance = maps.Clone(other.Provenance)
	}
	pulled := make(map[string]Metric[T], len(keys))
	for _, key := range keys {
		if metric, ok := other.Metrics[key]; ok {
			pulled[key] = metric
		}
	}
	other.mu.RUnlock()

	measurement.mu.Lock()
	measurement.Label = label
	measurement.At = at
	measurement.From = from
	measurement.SeqIdx = seq
	measurement.Timestamp = timestamp
	if provenance != nil {
		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string, len(provenance))
		}
		maps.Copy(measurement.Provenance, provenance)
	}
	if len(pulled) > 0 {
		if measurement.Metrics == nil {
			measurement.Metrics = make(map[string]Metric[T], len(pulled))
		}
		for key, metric := range pulled {
			measurement.Metrics[key] = metric
		}
	}
	measurement.mu.Unlock()
}

/*
Standardize walks the measurement's metrics as pointers, so a standardization
stage can fill each metric's normalized and standardized forms in place.
*/
func (measurement *Measurement[T]) Standardize() iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var keys []string
		measurement.mu.RLock()
		for key := range measurement.Metrics {
			keys = append(keys, key)
		}
		measurement.mu.RUnlock()

		for _, key := range keys {
			metric := measurement.GetMetric(key)

			if !yield(unsafe.Pointer(&metric)) {
				return
			}

			measurement.SetMetric(key, metric)
		}
	}
}

/*
Reset zeroes every metric's values in place, so a pre-allocated measurement
can flow through again without being reallocated. The declared schema never
moves.
*/
func (measurement *Measurement[T]) Reset() {
	var keys []string
	measurement.mu.RLock()
	for key := range measurement.Metrics {
		keys = append(keys, key)
	}
	measurement.mu.RUnlock()

	for _, key := range keys {
		metric := measurement.GetMetric(key)
		metric.Raw = zero[T]()
		metric.Normalized = nil
		metric.Standardized = nil
		measurement.SetMetric(key, metric)
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
qualityOf reads the measurement's own quality facts as one reading.
*/
func qualityOf[T any](measurement *Measurement[T]) QualityReading {
	if measurement == nil {
		return QualityReading{}
	}

	measurement.mu.RLock()
	defer measurement.mu.RUnlock()

	return QualityReading{
		SNR:        measurement.SNR,
		SNRDefined: measurement.SNRDefined,
		Estimated:  measurement.Estimated,
		Maturity:   measurement.Maturity,
	}
}

/*
Finalize derives the measurement's quality facts from its own estimator
metadata, mutating the measurement in place.
*/
func (measurement *Measurement[Value]) Finalize() {
	finalizer := NewFinalizer[Value]()
	held := measurement

	for range finalizer.Next(transport.NewOne(unsafe.Pointer(&held)).Next(nil)) {
	}
}

/*
Finalizer derives Maturity and SNR from each arriving measurement's own
estimator facts, mutating the measurement in place.
*/
type Finalizer[Value any] struct {
	err     error
	quality core.Primitive
}

/*
NewFinalizer creates the measurement quality derivation primitive.
*/
func NewFinalizer[Value any]() core.Primitive {
	return &Finalizer[Value]{quality: NewQuality()}
}

func (op *Finalizer[Value]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**Measurement[Value])(arriving)

			if measurement != nil {
				updates := make(map[string]Metric[Value])
				measurement.RangeMetrics(func(key string, metric Metric[Value]) bool {
					val, valid := any(metric.Raw).(float64)
					if !valid {
						return true
					}

					modified := false

					// Extract baseline as Center and stddev as Scale from estimator evidence
					if baseline, ok := measurement.LookupMetric(key + "_baseline"); ok && baseline.Label != "" {
						if bVal, ok := any(baseline.Raw).(float64); ok {
							metric.Center = bVal
							modified = true
							if zscore, ok := measurement.LookupMetric(key + "_zscore"); ok && zscore.Label != "" {
								if zVal, ok := any(zscore.Raw).(float64); ok && zVal != 0 {
									metric.Scale = math.Abs((val - bVal) / zVal)
								}
							}
						}
					}

					if modified {
						updates[key] = metric
					}
					return true
				})

				for k, v := range updates {
					v = v.Write(v.Raw)
					measurement.SetMetric(k, v)
				}

				readingEval := transport.NewEvaluate(op.quality)
				var reading QualityReading

				for out := range readingEval.Next(transport.NewValues(measurement.Facts()).Next(nil)) {
					reading = *(*QualityReading)(out)
				}

				err := readingEval.Error()

				if err != nil && measurement.Err == nil {
					measurement.Err = err
				}

				measurement.SetQuality(
					reading.Maturity, reading.SNR, reading.SNRDefined, reading.Estimated,
				)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Finalizer[Value]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

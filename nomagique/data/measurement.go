package data

import (
	"errors"
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
MetricEntry stores a single metric by its producer key.
*/
type MetricEntry[T any] struct {
	Key    string    `json:"key"`
	Metric Metric[T] `json:"metric"`
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
Under the WORM model:
  - Before publication: exactly one writer (the producer), zero readers.
  - After publication: zero writers, arbitrary concurrent readers.
No mutexes or synchronization primitives are used inside Measurement.
Hot data is stored in compact arena-backed slices instead of GC heap maps.
*/
type Measurement[T any] struct {
	ID         int               `json:"id"`
	Label      string            `json:"label"`
	Source     string            `json:"source"`
	SeqIdx     int64             `json:"seqIdx"`
	Timestamp  int64             `json:"timestamp"`
	At         time.Time         `json:"at"`
	From       time.Time         `json:"from,omitempty"`
	Maturity   float64           `json:"maturity"`
	SNR        float64           `json:"snr"`
	SNRDefined bool              `json:"snrDefined"`
	Estimated  bool              `json:"estimated"`
	Err        error             `json:"-"`
	Metrics    []MetricEntry[T]  `json:"metrics,omitempty"`
	Metadata   []StringEntry     `json:"metadata,omitempty"`
	Provenance []StringEntry     `json:"provenance,omitempty"`
	Peers      []*Measurement[T] `json:"peers,omitempty"`
	Result     any               `json:"-"`
}

func (m *Measurement[T]) localMetric(key string) (Metric[T], bool) {
	for i := range m.Metrics {
		if m.Metrics[i].Key == key {
			return m.Metrics[i].Metric, true
		}
	}
	return Metric[T]{}, false
}

// GetMetric retrieves a metric, checking local Metrics first, then direct Peers.
func (m *Measurement[T]) GetMetric(key string) Metric[T] {
	val, _ := m.LookupMetric(key)
	return val
}

// LookupMetric searches for a metric in local storage and direct Peers deterministically.
func (m *Measurement[T]) LookupMetric(key string) (Metric[T], bool) {
	if m == nil {
		return Metric[T]{}, false
	}

	if val, ok := m.localMetric(key); ok {
		return val, true
	}

	for _, peer := range m.Peers {
		if peer == nil {
			continue
		}

		if val, ok := peer.localMetric(key); ok {
			return val, true
		}

		for _, sub := range peer.Peers {
			if sub == nil {
				continue
			}

			if val, ok := sub.localMetric(key); ok {
				return val, true
			}
		}
	}

	return Metric[T]{}, false
}

// LookupPeerMetric searches for a metric originating from a specific source among Peers.
func (m *Measurement[T]) LookupPeerMetric(source, key string) (Metric[T], bool) {
	if m == nil {
		return Metric[T]{}, false
	}

	for _, peer := range m.Peers {
		if peer == nil {
			continue
		}

		if peer.Source == source {
			if val, ok := peer.localMetric(key); ok {
				return val, true
			}
		}

		for _, sub := range peer.Peers {
			if sub != nil && sub.Source == source {
				if val, ok := sub.localMetric(key); ok {
					return val, true
				}
			}
		}
	}

	return Metric[T]{}, false
}

// SetMetric sets a metric owned locally by this producer.
func (m *Measurement[T]) SetMetric(key string, val Metric[T]) {
	if m == nil {
		return
	}

	for i := range m.Metrics {
		if m.Metrics[i].Key == key {
			m.Metrics[i].Metric = val
			return
		}
	}

	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: val})
}

// WriteMetric safely writes a raw value to a local metric.
func (m *Measurement[T]) WriteMetric(key string, val T) {
	if m == nil {
		return
	}

	for i := range m.Metrics {
		if m.Metrics[i].Key == key {
			m.Metrics[i].Metric = m.Metrics[i].Metric.Write(val)
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric.Write(val)})
}

// WriteStandardized sets the raw value and standardized form on a local metric.
func (m *Measurement[T]) WriteStandardized(key string, val T) {
	if m == nil {
		return
	}

	v := val
	for i := range m.Metrics {
		if m.Metrics[i].Key == key {
			m.Metrics[i].Metric = m.Metrics[i].Metric.Write(val)
			m.Metrics[i].Metric.Standardized = &v
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	metric = metric.Write(val)
	metric.Standardized = &v
	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric})
}

// WriteNormalized sets the raw value and normalized form on a local metric.
func (m *Measurement[T]) WriteNormalized(key string, val T) {
	if m == nil {
		return
	}

	v := val
	for i := range m.Metrics {
		if m.Metrics[i].Key == key {
			m.Metrics[i].Metric = m.Metrics[i].Metric.Write(val)
			m.Metrics[i].Metric.Normalized = &v
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	metric = metric.Write(val)
	metric.Normalized = &v
	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric})
}

// RangeMetrics iterates over all local metrics.
func (m *Measurement[T]) RangeMetrics(f func(key string, metric Metric[T]) bool) {
	if m == nil {
		return
	}

	for _, entry := range m.Metrics {
		if !f(entry.Key, entry.Metric) {
			return
		}
	}
}

// GetSource returns Source directly without synchronization.
func (m *Measurement[T]) GetSource() string {
	if m == nil {
		return ""
	}
	return m.Source
}

// SetSource sets Source directly without synchronization.
func (m *Measurement[T]) SetSource(source string) {
	if m == nil {
		return
	}
	m.Source = source
}

// SetQuality sets quality indicators on the measurement.
func (m *Measurement[T]) SetQuality(maturity, snr float64, snrDefined, estimated bool) {
	if m == nil {
		return
	}

	m.Maturity = maturity
	m.SNR = snr
	m.SNRDefined = snrDefined
	m.Estimated = estimated
}

func (m *Measurement[T]) EnsureMetadata() {
	if m != nil && m.Metadata == nil {
		m.Metadata = make([]StringEntry, 0, 8)
	}
}

func (m *Measurement[T]) GetMetadata(key string) (string, bool) {
	if m == nil {
		return "", false
	}

	for _, entry := range m.Metadata {
		if entry.Key == key {
			return entry.Value, true
		}
	}

	return "", false
}

func (m *Measurement[T]) SetMetadata(key, value string) {
	if m == nil {
		return
	}

	for i := range m.Metadata {
		if m.Metadata[i].Key == key {
			m.Metadata[i].Value = value
			return
		}
	}

	m.Metadata = append(m.Metadata, StringEntry{Key: key, Value: value})
}

func (m *Measurement[T]) DeleteMetadata(key string) {
	if m == nil {
		return
	}

	for i := range m.Metadata {
		if m.Metadata[i].Key == key {
			m.Metadata = append(m.Metadata[:i], m.Metadata[i+1:]...)
			return
		}
	}
}

func (m *Measurement[T]) RangeMetadata(f func(key, value string) bool) {
	if m == nil {
		return
	}

	for _, entry := range m.Metadata {
		if !f(entry.Key, entry.Value) {
			return
		}
	}
}

func (m *Measurement[T]) EnsureProvenance() {
	if m != nil && m.Provenance == nil {
		m.Provenance = make([]StringEntry, 0, 8)
	}
}

func (m *Measurement[T]) GetProvenance(key string) (string, bool) {
	if m == nil {
		return "", false
	}

	for _, entry := range m.Provenance {
		if entry.Key == key {
			return entry.Value, true
		}
	}

	return "", false
}

func (m *Measurement[T]) SetProvenance(key, value string) {
	if m == nil {
		return
	}

	for i := range m.Provenance {
		if m.Provenance[i].Key == key {
			m.Provenance[i].Value = value
			return
		}
	}

	m.Provenance = append(m.Provenance, StringEntry{Key: key, Value: value})
}

func (m *Measurement[T]) DeleteProvenance(key string) {
	if m == nil {
		return
	}

	for i := range m.Provenance {
		if m.Provenance[i].Key == key {
			m.Provenance = append(m.Provenance[:i], m.Provenance[i+1:]...)
			return
		}
	}
}

func (m *Measurement[T]) RangeProvenance(f func(key, value string) bool) {
	if m == nil {
		return
	}

	for _, entry := range m.Provenance {
		if !f(entry.Key, entry.Value) {
			return
		}
	}
}

func (m *Measurement[T]) Facts() QualityFacts {
	if m == nil {
		return QualityFacts{}
	}

	snap := make(map[string]string, len(m.Metadata))
	for _, entry := range m.Metadata {
		snap[entry.Key] = entry.Value
	}

	return factsFromMetadata(snap)
}

func (measurement *Measurement[T]) SetSeqIdx(seq int64) {
	if measurement != nil {
		measurement.SeqIdx = seq
	}
}

func (measurement *Measurement[T]) Identity() int {
	if measurement == nil {
		return -1
	}
	return measurement.ID
}

func (measurement *Measurement[T]) Identify(id int) Identifiable[T] {
	if measurement != nil {
		measurement.ID = id
	}
	return measurement
}

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
	source string, initialMetrics ...map[string]Metric[T],
) *Measurement[T] {
	var metrics []MetricEntry[T]
	if len(initialMetrics) > 0 && initialMetrics[0] != nil {
		metrics = make([]MetricEntry[T], 0, len(initialMetrics[0]))
		for k, v := range initialMetrics[0] {
			metrics = append(metrics, MetricEntry[T]{Key: k, Metric: v})
		}
	}

	return &Measurement[T]{
		ID:         -1,
		Source:     source,
		Metrics:    metrics,
		Metadata:   make([]StringEntry, 0, 8),
		Provenance: make([]StringEntry, 0, 8),
		Peers:      make([]*Measurement[T], 0, 4),
	}
}



const (
	MetadataSupport        = "support"
	MetadataMaturity       = "maturity"
	MetadataDivergence     = "divergence"
	MetadataNoiseVariance  = "noise_variance"
	MetadataMahalanobisSNR = "mahalanobis_snr"
)

func (measurement *Measurement[Value]) Finalize() {
	finalizer := NewFinalizer[Value]()
	held := measurement

	for range finalizer.Next(transport.NewOne(unsafe.Pointer(&held)).Next(nil)) {
	}
}

type Finalizer[Value any] struct {
	err     error
	quality core.Primitive
}

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

func (op *Finalizer[Value]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

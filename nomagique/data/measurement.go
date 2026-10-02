package data

import (
	"errors"
	"iter"
	"math"
	"strings"
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
	Epoch      int64             `json:"epoch"`
	Label      string            `json:"label"`
	Source     string            `json:"source"`
	SeqIdx     int64             `json:"seqIdx"`
	Timestamp  int64             `json:"timestamp"`
	At         time.Time         `json:"at"`
	From       time.Time         `json:"from"`
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

	for index := range m.Metrics {
		if m.Metrics[index].Key == key {
			if m.Metrics[index].Metric.Label == "" {
				m.Metrics[index].Metric.Label = key
			}

			m.Metrics[index].Metric = m.Metrics[index].Metric.Write(val)
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	if metric.Label == "" {
		metric.Label = key
	}

	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric.Write(val)})
}

// WriteStandardized sets the raw value and standardized form on a local metric.
func (m *Measurement[T]) WriteStandardized(key string, val T) {
	if m == nil {
		return
	}

	stdVal := val
	for index := range m.Metrics {
		if m.Metrics[index].Key == key {
			if m.Metrics[index].Metric.Label == "" {
				m.Metrics[index].Metric.Label = key
			}

			m.Metrics[index].Metric = m.Metrics[index].Metric.Write(val)
			m.Metrics[index].Metric.Standardized = &stdVal
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	if metric.Label == "" {
		metric.Label = key
	}

	metric = metric.Write(val)
	metric.Standardized = &stdVal
	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric})
}

// WriteNormalized sets the raw value and normalized form on a local metric.
func (m *Measurement[T]) WriteNormalized(key string, val T) {
	if m == nil {
		return
	}

	normVal := val
	for index := range m.Metrics {
		if m.Metrics[index].Key == key {
			if m.Metrics[index].Metric.Label == "" {
				m.Metrics[index].Metric.Label = key
			}

			m.Metrics[index].Metric = m.Metrics[index].Metric.Write(val)
			m.Metrics[index].Metric.Normalized = &normVal
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	if metric.Label == "" {
		metric.Label = key
	}

	metric = metric.Write(val)
	metric.Normalized = &normVal
	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric})
}

// SetCenterScale sets the center and scale on a metric and immediately re-evaluates its standardization.
func (m *Measurement[T]) SetCenterScale(key string, center, scale float64) {
	if m == nil {
		return
	}

	for index := range m.Metrics {
		if m.Metrics[index].Key == key {
			if m.Metrics[index].Metric.Label == "" {
				m.Metrics[index].Metric.Label = key
			}

			m.Metrics[index].Metric.Center = center
			m.Metrics[index].Metric.Scale = scale
			m.Metrics[index].Metric = m.Metrics[index].Metric.Write(m.Metrics[index].Metric.Raw)
			return
		}
	}

	metric, _ := m.LookupMetric(key)
	if metric.Label == "" {
		metric.Label = key
	}

	metric.Center = center
	metric.Scale = scale
	m.Metrics = append(m.Metrics, MetricEntry[T]{Key: key, Metric: metric.Write(metric.Raw)})
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
				var midpoint, spread float64
				var hasMid, hasSpread bool

				if midMetric, ok := measurement.LookupMetric("midpoint"); ok {
					if midVal, valid := any(midMetric.Raw).(float64); valid && midVal > 0 {
						midpoint = midVal
						hasMid = true
					}
				}

				if spreadMetric, ok := measurement.LookupMetric("spread"); ok {
					if spreadVal, valid := any(spreadMetric.Raw).(float64); valid && spreadVal > 0 {
						spread = spreadVal
						hasSpread = true
					}
				}

				if !hasMid || !hasSpread {
					var bid, ask float64
					var hasBid, hasAsk bool

					for _, bidKey := range []string{"best_bid", "bid", "best_bid_price", "bid_price"} {
						if bidMetric, ok := measurement.LookupMetric(bidKey); ok {
							if bidVal, valid := any(bidMetric.Raw).(float64); valid && bidVal > 0 {
								bid = bidVal
								hasBid = true
								break
							}
						}
					}

					for _, askKey := range []string{"best_ask", "ask", "best_ask_price", "ask_price"} {
						if askMetric, ok := measurement.LookupMetric(askKey); ok {
							if askVal, valid := any(askMetric.Raw).(float64); valid && askVal > 0 {
								ask = askVal
								hasAsk = true
								break
							}
						}
					}

					if hasBid && hasAsk && ask > bid {
						if !hasMid {
							midpoint = (bid + ask) / 2.0
							hasMid = true
						}

						if !hasSpread {
							spread = ask - bid
							hasSpread = true
						}
					}
				}

				updates := make(map[string]Metric[Value])
				var zScores []float64

				measurement.RangeMetrics(func(key string, metric Metric[Value]) bool {
					if metric.Label == "" {
						metric.Label = key
					}

					val, valid := any(metric.Raw).(float64)
					if !valid {
						return true
					}

					modified := false

					if isPriceMetric(key) && hasMid && hasSpread && spread > 0 {
						if metric.Center == 0 && metric.Scale == 0 {
							metric.Center = midpoint
							metric.Scale = spread
							modified = true
						}
					}

					if baseline, ok := measurement.LookupMetric(key + "_baseline"); ok {
						if bVal, ok := any(baseline.Raw).(float64); ok {
							metric.Center = bVal
							modified = true

							if zscore, ok := measurement.LookupMetric(key + "_zscore"); ok {
								if zVal, ok := any(zscore.Raw).(float64); ok && zVal != 0 {
									metric.Scale = math.Abs(val-bVal) / math.Abs(zVal)
								}
							}

							if metric.Scale == 0 {
								if noiseScale, ok := measurement.LookupMetric(key + "_noise_scale"); ok {
									if nVal, ok := any(noiseScale.Raw).(float64); ok && nVal > 0 {
										metric.Scale = nVal
									}
								}
							}

							if metric.Scale == 0 {
								if noiseVar, ok := measurement.LookupMetric(key + "_noise_variance"); ok {
									if vVal, ok := any(noiseVar.Raw).(float64); ok && vVal > 0 {
										metric.Scale = math.Sqrt(vVal)
									}
								}
							}
						}
					}

					if isBoundedMetric(key) {
						if metric.Normalized == nil {
							normVal := val
							metric.Normalized = any(&normVal).(*Value)
							modified = true
						}

						if metric.Center == 0 && metric.Scale == 0 {
							metric.Center = 0
							metric.Scale = 1
							modified = true
						}
					}

					if strings.HasSuffix(key, "_zscore") {
						if !math.IsNaN(val) && !math.IsInf(val, 0) {
							zScores = append(zScores, val)
						}

						if metric.Standardized == nil {
							stdVal := val
							metric.Standardized = any(&stdVal).(*Value)
							modified = true
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

				snr := reading.SNR
				snrDefined := reading.SNRDefined

				if !snrDefined && len(zScores) > 0 {
					var sumSquares float64
					for _, zScoreVal := range zScores {
						sumSquares += zScoreVal * zScoreVal
					}
					snr = sumSquares / float64(len(zScores))
					snrDefined = true
				}

				if !snrDefined {
					facts := measurement.Facts()
					if facts.HasDivergence && facts.HasNoise && facts.NoiseVariance > 0 {
						snr = (facts.Divergence * facts.Divergence) / facts.NoiseVariance
						snrDefined = true
					}
				}

				maturity := reading.Maturity
				if maturity == 0 {
					facts := measurement.Facts()
					if facts.HasSupport && facts.Support > 1 {
						maturity = 1.0 - (1.0 / facts.Support)
					}

					if maturity == 0 && !reading.Estimated {
						maturity = 1.0
					}
				}

				measurement.SetQuality(
					maturity, snr, snrDefined, reading.Estimated,
				)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func isPriceMetric(key string) bool {
	switch key {
	case "price", "bid", "ask", "best_bid", "best_ask", "best_bid_price", "best_ask_price",
		"bid_price", "ask_price", "last", "last_price", "derivative_price", "reference_price",
		"response_midpoint:at", "response_midpoint:from", "midpoint":
		return true
	}

	return false
}

func isBoundedMetric(key string) bool {
	if strings.Contains(key, "imbalance") || strings.HasSuffix(key, "_fraction") ||
		strings.Contains(key, "signed_correlation") || strings.Contains(key, "absolute_correlation") ||
		strings.HasSuffix(key, "_ks") || strings.HasSuffix(key, "_percentile") {
		return true
	}

	return false
}

func (op *Finalizer[Value]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

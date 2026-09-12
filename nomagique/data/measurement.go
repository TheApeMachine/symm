package data

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Measurement is the projected output of a pipeline: one identified observation
with provenance, timing, quality, and its metric projections. It carries no
market semantics — Label names what was measured, Source names what produced
it, and both are plain strings.

Quality is not caller-supplied. Maturity and SNR are derived by the Finalizer
primitive from the measurement's own estimator facts, so no later step can
fake or reward-hack them by writing the fields directly.
*/
type Measurement[T any] struct {
	ID         int                  `json:"id"`
	Label      string               `json:"label"`
	Source     string               `json:"source"`
	SeqIdx     int64                `json:"seqIdx"`
	At         time.Time            `json:"at"`
	From       time.Time            `json:"from,omitempty"`
	Maturity   float64              `json:"maturity"`
	SNR        float64              `json:"snr"`
	SNRDefined bool                 `json:"snrDefined"`
	Estimated  bool                 `json:"estimated"`
	Err        error                `json:"-"`
	Metrics    map[string]Metric[T] `json:"metrics,omitempty"`
	Metadata   map[string]float64   `json:"metadata,omitempty"`
	Provenance map[string]string    `json:"provenance,omitempty"`
	Peers      []*Measurement[T]    `json:"peers"`
}

/*
NewMeasurement creates one identified observation with empty metric storage.
Its identity is unstamped: the register slot that owns it is assigned when a
workload registers it, and the node stamps the slot back via SetID.
*/
func NewMeasurement[T any](
	source string, metrics map[string]Metric[T],
) *Measurement[T] {
	return &Measurement[T]{
		ID:      -1,
		Source:  source,
		Metrics: metrics,
	}
}

func (measurement *Measurement[T]) Identify() int {
	return measurement.ID
}

/*
Standardize walks the measurement's metrics as pointers, so a standardization
stage can fill each metric's normalized and standardized forms in place.
*/
func (measurement *Measurement[T]) Standardize() iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for key := range measurement.Metrics {
			metric := measurement.Metrics[key]

			if !yield(unsafe.Pointer(&metric)) {
				return
			}

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
qualityOf reads the measurement's own quality facts as one reading.
*/
func qualityOf[T any](measurement *Measurement[T]) QualityReading {
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

			if measurement != nil && measurement.Err == nil {
				readingEval := transport.NewEvaluate(op.quality)
				var reading QualityReading

				for out := range readingEval.Next(transport.NewValues(factsFromMetadata(measurement.Metadata)).Next(nil)) {
					reading = *(*QualityReading)(out)
				}

				err := readingEval.Error()

				if err != nil {
					measurement.Err = err
				}

				measurement.Maturity = reading.Maturity
				measurement.SNR = reading.SNR
				measurement.SNRDefined = reading.SNRDefined
				measurement.Estimated = reading.Estimated
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

/*
Cloner yields one independent deep copy of each arriving measurement and its
mappings.
*/
type Cloner[Value any] struct {
	err error
	out *Measurement[Value]
}

/*
NewCloner creates the measurement deep-copy primitive.
*/
func NewCloner[Value any]() core.Primitive {
	return &Cloner[Value]{}
}

func (op *Cloner[Value]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**Measurement[Value])(arriving)

			if measurement == nil {
				op.out = nil

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			metrics := make(map[string]Metric[Value], len(measurement.Metrics))

			for key, val := range measurement.Metrics {
				metrics[key] = val
			}

			var metadata map[string]float64

			if measurement.Metadata != nil {
				metadata = make(map[string]float64, len(measurement.Metadata))

				for key, val := range measurement.Metadata {
					metadata[key] = val
				}
			}

			var provenance map[string]string

			if measurement.Provenance != nil {
				provenance = make(map[string]string, len(measurement.Provenance))

				for key, val := range measurement.Provenance {
					provenance[key] = val
				}
			}

			op.out = &Measurement[Value]{
				ID:         measurement.ID,
				Label:      measurement.Label,
				Source:     measurement.Source,
				SeqIdx:     measurement.SeqIdx,
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
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Cloner[Value]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

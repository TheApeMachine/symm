package data

import (
	"strings"
	"time"
)

/*
Measurement is the projected output of a pipeline: one identified observation
with provenance, timing, quality, and its metric projections. It carries no
market semantics — Label names what was measured, Source names what produced
it, and both are plain strings.

Quality is not caller-supplied. Maturity and SNR are derived inside Finalize
from the measurement's own estimator facts, so no later step can fake or
reward-hack them by writing the fields directly.
*/
type Measurement[Value any] struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Source   string    `json:"source"`
	SeqIdx   int64     `json:"seqIdx"`
	At       time.Time `json:"at"`
	From     time.Time `json:"from,omitempty"`
	Maturity float64   `json:"maturity"`
	SNR      float64   `json:"snr"`
	// SNRDefined distinguishes a measured SNR (including a genuine zero
	// departure) from an undefined SNR where no noise model was estimable.
	SNRDefined bool                     `json:"snrDefined"`
	Estimated  bool                     `json:"estimated"`
	Err        error                    `json:"-"`
	Metrics    map[string]Metric[Value] `json:"metrics,omitempty"`
	Metadata   map[string]float64       `json:"metadata,omitempty"`
	Provenance map[string]string        `json:"provenance,omitempty"`
}

func (measurement *Measurement[Value]) ExecutionKey() string {
	if measurement == nil {
		return "global"
	}

	return measurement.Label
}

func (measurement *Measurement[Value]) Symbol() string {
	if measurement == nil {
		return ""
	}

	return measurement.Label
}

func NewMeasurement[Value any](id, label, source string, at, from time.Time) *Measurement[Value] {
	return &Measurement[Value]{
		ID:      id,
		Label:   label,
		Source:  source,
		At:      at,
		From:    from,
		Metrics: make(map[string]Metric[Value]),
	}
}

/* Clone returns an independent deep copy of the measurement and its mappings. */
func (measurement *Measurement[Value]) Clone() *Measurement[Value] {
	if measurement == nil {
		return nil
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

	return &Measurement[Value]{
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
}

func (measurement *Measurement[Value]) PutMetric(metric Metric[Value]) {
	if measurement == nil || metric.Label == "" {
		return
	}

	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]Metric[Value])
	}

	measurement.Metrics[metric.Label] = metric
}

const (
	MetadataSupport        = "support"
	MetadataMaturity       = "maturity"
	MetadataDivergence     = "divergence"
	MetadataNoiseVariance  = "noise_variance"
	MetadataMahalanobisSNR = "mahalanobis_snr"
)

func (measurement *Measurement[Value]) quality() QualityReading {
	return QualityReading{
		SNR:        measurement.SNR,
		SNRDefined: measurement.SNRDefined,
		Estimated:  measurement.Estimated,
		Maturity:   measurement.Maturity,
	}
}

/*
Finalize derives Maturity and SNR from the measurement's own estimator facts.
*/
func (measurement *Measurement[Value]) Finalize() {
	if measurement == nil || measurement.Err != nil {
		return
	}

	reading, err := NewQuality().Derive(factsFromMetadata(measurement.Metadata))

	if err != nil {
		measurement.Err = err
		return
	}

	measurement.Maturity = reading.Maturity
	measurement.SNR = reading.SNR
	measurement.SNRDefined = reading.SNRDefined
	measurement.Estimated = reading.Estimated
}

/* Authority reads the canonical evidence-authority equation. */
func (measurement *Measurement[Value]) Authority() float64 {
	if measurement == nil {
		return 0
	}

	if measurement.Maturity == 0 && !measurement.SNRDefined && !measurement.Estimated {
		measurement.Finalize()
	}

	if measurement.Err != nil {
		return 0
	}

	value, err := NewAuthority().Weight(measurement.quality())

	if err != nil {
		measurement.Err = err
		return 0
	}

	return value
}

/*
Readout projects one metric into the usable authority-weighted observation.
*/
func (measurement *Measurement[Value]) Readout(label string) *Readout {
	if measurement == nil || measurement.Err != nil {
		return nil
	}

	metric, found := measurement.Metrics[label]

	if !found {
		return nil
	}

	if measurement.Maturity == 0 && !measurement.SNRDefined && !measurement.Estimated {
		measurement.Finalize()
	}

	if measurement.Err != nil {
		return nil
	}

	raw, valid := any(metric.Raw).(float64)

	if !valid {
		return nil
	}

	reading, err := NewReadout().Resolve(ReadoutInput{
		QualityReading: measurement.quality(),
		Raw:            raw,
		Credibility:    1,
		Defined:        true,
		Discrete:       metric.Unit == UnitCount || strings.Contains(label, "ordinal"),
	})

	if err != nil {
		measurement.Err = err
		return nil
	}

	return &reading
}

/* Readouts retains each metric's explicit readout. */
func (measurement *Measurement[Value]) Readouts() map[string]Readout {
	if measurement == nil {
		return nil
	}

	readouts := make(map[string]Readout, len(measurement.Metrics))

	for label := range measurement.Metrics {
		if readout := measurement.Readout(label); readout != nil {
			readouts[label] = *readout
		}
	}

	return readouts
}

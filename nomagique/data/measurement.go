package data

import (
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"strings"
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

/*
NewMeasurement builds an empty projection with its metrics map allocated.
*/
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

/*
PutMetric stores one projected metric under its own label.
*/
func (measurement *Measurement[Value]) PutMetric(metric Metric[Value]) {
	if measurement == nil || metric.Label == "" {
		return
	}

	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]Metric[Value])
	}

	measurement.Metrics[metric.Label] = metric
}

/*
QualityFact names the raw estimator facts a pipeline must carry as metadata so
Finalize can derive Maturity and SNR without any caller-supplied numbers.
*/
const (
	MetadataSupport        = "support"
	MetadataDivergence     = "divergence"
	MetadataNoiseVariance  = "noise_variance"
	MetadataMahalanobisSNR = "mahalanobis_snr"
)

/*
Finalize derives Maturity and SNR from the measurement's own estimator facts.

Maturity follows the global spec (§8): effective support N maps to
1 - 1/N when N > 1, otherwise 0. A stateless direct measurement with no
historical estimator carries no support slot and is whole (Maturity 1).

SNR follows spec §7:
- Multivariate Mahalanobis SNR (§7.2): (1/k) * delta^T * Sigma^-1 * delta
- Scalar SNR (§7.1): divergence^2 / noise_variance

When no noise model or covariance is estimable the SNR is undefined (reported as
zero and left distinguishable from a genuine zero departure by the absence of the
noise or covariance fact).
*/
func (measurement *Measurement[Value]) Finalize() {
	if measurement == nil || measurement.Err != nil {
		return
	}
	facts := make(map[string]core.Primitive, len(measurement.Metadata))
	for name, value := range measurement.Metadata {
		facts[name] = core.From(value)
	}
	graphs := projectionPool.Get().(*projectionGraphs)
	fields, err := transport.Evaluate[map[string]core.Primitive](graphs.quality, core.From(facts))
	if err != nil {
		measurement.Err = err
		return
	}
	decoder := core.NewDecoder(fields)
	measurement.Maturity = core.Decode[float64](decoder, "maturity")
	measurement.SNR = core.Decode[float64](decoder, "snr")
	measurement.SNRDefined = core.Decode[bool](decoder, "snr_defined")
	measurement.Estimated = core.Decode[bool](decoder, "estimated")
	measurement.Err = decoder.Error()
	if measurement.Err == nil {
		projectionPool.Put(graphs)
	}
}

/* Authority reads the canonical Primitive authority equation, never a duplicate formula. */
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
	graphs := projectionPool.Get().(*projectionGraphs)
	authority, err := transport.Evaluate[float64](graphs.authority, core.Record(map[string]any{
		"maturity": measurement.Maturity, "snr": measurement.SNR,
		"snr_defined": measurement.SNRDefined, "estimated": measurement.Estimated,
	}))
	if err != nil {
		measurement.Err = err
		return 0
	}
	projectionPool.Put(graphs)
	return authority
}

/*
Readout projects one metric into the Primitive record used by NewReadout.
Unit, timestamps and optional normalized/standardized values remain provenance;
no former Readout receiver or hidden graph traversal is recreated.
*/
func (measurement *Measurement[Value]) Readout(label string) core.Primitive {
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
		measurement.Err = core.ErrWrongType
		return nil
	}
	record := core.Record(map[string]any{
		"source": measurement.Source, "label": label, "raw": raw,
		"maturity": measurement.Maturity, "snr": measurement.SNR,
		"snr_defined": measurement.SNRDefined, "estimated": measurement.Estimated,
		"at": measurement.At.UnixNano(), "unit": string(metric.Unit), "timescale": string(metric.Timescale),
		"discrete": metric.Unit == UnitCount || strings.Contains(label, "ordinal"),
	})
	graphs := projectionPool.Get().(*projectionGraphs)
	fields, err := transport.Evaluate[map[string]core.Primitive](graphs.readout, record)
	if err != nil {
		measurement.Err = err
		return nil
	}
	if metric.Normalized != nil {
		fields["normalized"] = core.From(*metric.Normalized)
	}
	if metric.Standardized != nil {
		fields["standardized"] = core.From(*metric.Standardized)
	}
	projectionPool.Put(graphs)
	return core.From(fields)
}

/* Readouts retains each metric's explicit Primitive result record. */
func (measurement *Measurement[Value]) Readouts() map[string]core.Primitive {
	if measurement == nil {
		return nil
	}
	readouts := make(map[string]core.Primitive, len(measurement.Metrics))
	for label := range measurement.Metrics {
		if readout := measurement.Readout(label); readout != nil {
			readouts[label] = readout
		}
	}
	return readouts
}

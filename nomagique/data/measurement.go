package data

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
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
ToTypesMeasurement converts a projected data.Measurement[float64] into a *types.Measurement.
*/
func (measurement *Measurement[Value]) ToTypesMeasurement() *types.Measurement {
	if measurement == nil {
		return nil
	}

	metrics := make(map[string]*types.Metric[float64], len(measurement.Metrics))

	for name, m := range measurement.Metrics {
		if floatVal, ok := any(m.Raw).(float64); ok {
			metric := types.NewMetric(name, floatVal, types.Descriptor{
				Unit:      types.ParseUnit(string(m.Unit)),
				Timescale: types.ParseTimescale(string(m.Timescale)),
			})

			if m.Normalized != nil {
				if normVal, normOk := any(*m.Normalized).(float64); normOk {
					metric.Normalized = &normVal
				}
			}

			metrics[name] = metric
		}
	}

	id := measurement.ID

	if id == "" {
		id = fmt.Sprintf("%s:%s:%d", measurement.Source, measurement.Label, measurement.At.UnixNano())
	}

	return &types.Measurement{
		ID:           id,
		Source:       measurement.Source,
		Symbol:       measurement.Label,
		At:           measurement.At,
		ObservedFrom: measurement.From,
		Maturity:     measurement.Maturity,
		SNR:          measurement.SNR,
		SNRDefined:   measurement.SNRDefined,
		Metrics:      metrics,
		Metadata:     measurement.Metadata,
		Err:          measurement.Err,
	}
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
	if measurement == nil {
		return
	}

	if measurement.Metadata == nil {
		measurement.Metadata = map[string]float64{}
	}

	_, hasSupport := measurement.Metadata[MetadataSupport]
	_, hasDivergence := measurement.Metadata[MetadataDivergence]
	_, hasMahalanobis := measurement.Metadata[MetadataMahalanobisSNR]

	measurement.Estimated = hasSupport || hasDivergence || hasMahalanobis

	// Derive the scalar SNR first so even a stateless measurement with the
	// estimator facts still reports it, then fall back to Maturity handling.
	divergence, hasDivergenceVal := measurement.Metadata[MetadataDivergence]
	noiseVariance, hasNoise := measurement.Metadata[MetadataNoiseVariance]

	if hasDivergenceVal && hasNoise && noiseVariance > 0 {
		measurement.SNR = divergence * divergence / noiseVariance
		measurement.SNRDefined = true
	}

	support, hasSupportVal := measurement.Metadata[MetadataSupport]

	if !hasSupportVal {
		// Stateless direct measurement: whole.
		measurement.Maturity = 1
		return
	}

	if support <= 1 {
		measurement.Maturity = 0
		return
	}

	measurement.Maturity = 1 - 1/support

	if mahalanobisSNR, hasMahalanobisVal := measurement.Metadata[MetadataMahalanobisSNR]; hasMahalanobisVal && mahalanobisSNR >= 0 {
		measurement.SNR = mahalanobisSNR
		measurement.SNRDefined = true
	}
}

/*
Readout returns one metric from the measurement wrapped as a Readout with the
measurement's own maturity, SNR, and event timestamp.
*/
func (measurement *Measurement[Value]) Readout(label string) *Readout {
	if measurement == nil || measurement.Metrics == nil {
		return nil
	}

	if measurement.Maturity == 0 && !measurement.SNRDefined && !measurement.Estimated {
		measurement.Finalize()
	}

	metric, found := measurement.Metrics[label]

	if !found {
		return nil
	}

	var raw float64

	if floatVal, ok := any(metric.Raw).(float64); ok {
		raw = floatVal
	}

	readout := NewReadout(
		measurement.Source,
		label,
		raw,
		measurement.Maturity,
		measurement.SNR,
		measurement.SNRDefined,
		measurement.Estimated,
		measurement.At,
	)

	if metric.Normalized != nil {
		if normVal, ok := any(*metric.Normalized).(float64); ok {
			readout.Normalized = &normVal
		}
	}

	if metric.Standardized != nil {
		if stdVal, ok := any(*metric.Standardized).(float64); ok {
			readout.Standardized = &stdVal
		}
	}

	readout.Unit = metric.Unit
	readout.Timescale = metric.Timescale

	return readout
}

/*
Readouts returns all metrics of this measurement converted to Readouts.
*/
func (measurement *Measurement[Value]) Readouts() map[string]*Readout {
	if measurement == nil || measurement.Metrics == nil {
		return nil
	}

	readouts := make(map[string]*Readout, len(measurement.Metrics))

	for label := range measurement.Metrics {
		if r := measurement.Readout(label); r != nil {
			readouts[label] = r
		}
	}

	return readouts
}


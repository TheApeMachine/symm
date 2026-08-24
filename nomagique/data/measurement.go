package data

import "time"

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
	ID       string                   `json:"id"`
	Label    string                   `json:"label"`
	Source   string                   `json:"source"`
	SeqIdx   int64                    `json:"seqIdx"`
	At       time.Time                `json:"at"`
	From     time.Time                `json:"from,omitempty"`
	Maturity float64                  `json:"maturity"`
	SNR      float64                  `json:"snr"`
	Err      error                    `json:"-"`
	Metrics  map[string]Metric[Value] `json:"metrics,omitempty"`
	Metadata map[string]float64       `json:"metadata,omitempty"`
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
	MetadataSupport      = "support"
	MetadataDivergence   = "divergence"
	MetadataNoiseVariance = "noise_variance"
)

/*
Finalize derives Maturity and SNR from the measurement's own estimator facts.

Maturity follows the global spec (§8): effective support N maps to
1 - 1/N when N > 1, otherwise 0. A stateless direct measurement with no
historical estimator carries no support slot and is whole (Maturity 1).

SNR follows spec §7.1: divergence^2 / noise_variance, unbounded and
non-negative. When no noise model is estimable the noise_variance meta is
absent and SNR is undefined; it is reported as zero and left distinguishable
from a genuine zero departure by the absence of the noise_variance fact.
*/
func (measurement *Measurement[Value]) Finalize() {
	if measurement == nil {
		return
	}

	if measurement.Metadata == nil {
		measurement.Metadata = map[string]float64{}
	}

	support, hasSupport := measurement.Metadata[MetadataSupport]

	if !hasSupport {
		// Stateless direct measurement: whole.
		measurement.Maturity = 1
		return
	}

	if support <= 1 {
		measurement.Maturity = 0
	} else {
		measurement.Maturity = 1 - 1/support
	}

	divergence, hasDivergence := measurement.Metadata[MetadataDivergence]
	noiseVariance, hasNoise := measurement.Metadata[MetadataNoiseVariance]

	if hasDivergence && hasNoise && noiseVariance > 0 {
		measurement.SNR = divergence * divergence / noiseVariance
	}
}

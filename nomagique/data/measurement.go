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
	ID         string                   `json:"id"`
	Label      string                   `json:"label"`
	Source     string                   `json:"source"`
	SeqIdx     int64                    `json:"seqIdx"`
	At         time.Time                `json:"at"`
	From       time.Time                `json:"from,omitempty"`
	Maturity   float64                  `json:"maturity"`
	SNR        float64                  `json:"snr"`
	Err        error                    `json:"-"`
	Metrics    map[string]Metric[Value] `json:"metrics,omitempty"`
	Metadata   map[string]float64       `json:"metadata,omitempty"`
	Provenance map[string]string        `json:"provenance,omitempty"`
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

	snr := measurement.SNR
	metrics["snr"] = types.NewMetric("snr", snr, types.Descriptor{
		Unit: types.UnitDimensionless,
	})

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

	// Derive the scalar SNR first so even a stateless measurement with the
	// estimator facts still reports it, then fall back to Maturity handling.
	divergence, hasDivergence := measurement.Metadata[MetadataDivergence]
	noiseVariance, hasNoise := measurement.Metadata[MetadataNoiseVariance]

	if hasDivergence && hasNoise && noiseVariance > 0 {
		measurement.SNR = divergence * divergence / noiseVariance
	}

	support, hasSupport := measurement.Metadata[MetadataSupport]

	if !hasSupport {
		// Stateless direct measurement: whole.
		measurement.Maturity = 1
		return
	}

	if support <= 1 {
		measurement.Maturity = 0
		return
	}

	measurement.Maturity = 1 - 1/support

	if mahalanobisSNR, hasMahalanobis := measurement.Metadata[MetadataMahalanobisSNR]; hasMahalanobis && mahalanobisSNR >= 0 {
		measurement.SNR = mahalanobisSNR
	}
}

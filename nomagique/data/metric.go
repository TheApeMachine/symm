package data

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Metric is one projected value of a measurement: a label, the raw observation,
its normalized and standardized forms, and the physical unit and timescale.

Exact retains the venue's original decimal for observations the venue printed
exactly (prices, sizes). Ingest writes it and capture/audit reads it; the
mathematics runs on Raw alone and never consults it. Derived facts leave it
nil.
*/
type Metric struct {
	Label        string           // The name of the metric.
	center       float64          // The center of the metric.
	scale        float64          // The scale of the metric.
	Raw          float64          // The raw value of the metric.
	Normalized   float64          // The normalized value of the metric.
	Standardized float64          // The standardized value of the metric.
	Exact        *decimal.Decimal // The exact value of the metric.
	unit         Unit             // The unit of the metric.
	timescale    Timescale        // The timescale of the metric.
}

/*
NewMetric builds a metric declaring its label, unit, timescale, and the
center and scale its values are standardized against.
*/
func NewMetric(
	label string, raw float64, unit Unit, timescale Timescale,
) *Metric {
	canonicalUnit, canonicalTimescale := CanonicalDimensions(label, unit, timescale)

	return &Metric{
		Label:     label,
		Raw:       raw,
		unit:      canonicalUnit,
		timescale: canonicalTimescale,
	}
}

/*
NewExactMetric builds a metric declaring its label, unit, timescale, and the
exact value of the metric.
*/
func NewExactMetric(
	label string, exact *decimal.Decimal, unit Unit, timescale Timescale,
) *Metric {
	canonicalUnit, canonicalTimescale := CanonicalDimensions(label, unit, timescale)

	return &Metric{
		Label:     label,
		Raw:       exact.Float64(),
		Exact:     exact,
		unit:      canonicalUnit,
		timescale: canonicalTimescale,
	}
}

func (metric *Metric) Unit() Unit {
	if metric == nil {
		return ""
	}

	return CanonicalUnit(metric.Label, metric.unit)
}

func (metric *Metric) Timescale() Timescale {
	if metric == nil {
		return ""
	}

	return CanonicalTimescale(metric.Label, metric.timescale)
}

/*
finalize is called from the Measurement to standardize the observation against
its stream's causal moments: center and scale are the stream's mean and sample
standard deviation before this observation, in the stream's standardization
space (CanonicalScale: Raw itself, ln Raw, or its log-modulus), and
Standardized is (value - center) / scale for Raw's value in that space. While the prior scale is zero the z-score is undefined:
Standardized and Normalized stay zero and Standardizable reports false. An
invalid observation is rejected before it reaches the stream.
*/
func (metric *Metric) finalize(state *standardizer) error {
	if err := errnie.Require(map[string]any{
		"raw": metric.Raw,
	}); err != nil {
		return errnie.Error(err)
	}

	value, center, scale, defined := state.observe(metric.Raw, CanonicalScale(metric.Label))
	metric.center, metric.scale = center, scale
	metric.Standardized = 0
	metric.Normalized = 0

	if defined && metric.Standardizable() {
		metric.Standardized = (value - metric.center) / metric.scale
		metric.Normalized = math.Tanh(metric.Standardized)
	}

	return metric.valid()
}

/*
Standardizable reports whether Standardized holds a z-score. It is false until
the metric's stream has a positive causal scale; an undefined z-score is
missing evidence, not a zero deformation.
*/
func (metric *Metric) Standardizable() bool {
	return metric.scale > 0
}

/*
valid checks if the Measurement is valid.
*/
func (metric *Metric) valid() error {
	// Exact is optional by contract: only venue-printed observations carry
	// it, derived facts leave it nil.
	return errnie.Error(errnie.Require(map[string]any{
		"label":        metric.Label,
		"raw":          metric.Raw,
		"normalized":   metric.Normalized,
		"standardized": metric.Standardized,
		"center":       metric.center,
		"scale":        metric.scale,
		"unit":         metric.unit,
		"timescale":    metric.timescale,
		"updated":      true,
	}),
		"metric", metric.Label,
		"raw", metric.Raw,
		"center", metric.center,
		"scale", metric.scale,
	)
}

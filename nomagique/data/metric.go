package data

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
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
	return &Metric{
		Label:     label,
		Raw:       raw,
		unit:      CanonicalUnit(label, unit),
		timescale: timescale,
	}
}

/*
NewExactMetric builds a metric declaring its label, unit, timescale, and the
exact value of the metric.
*/
func NewExactMetric(
	label string, exact *decimal.Decimal, unit Unit, timescale Timescale,
) *Metric {
	return &Metric{
		Label:     label,
		Raw:       exact.Float64(),
		Exact:     exact,
		unit:      CanonicalUnit(label, unit),
		timescale: timescale,
	}
}

func (metric *Metric) Unit() Unit {
	if metric == nil {
		return ""
	}

	return CanonicalUnit(metric.Label, metric.unit)
}

func (metric *Metric) Timescale() Timescale {
	return metric.timescale
}

/*
finalize is called from the Measurement to set the derived values, like
center, scale, normalized, and standardized values. An invalid observation
is rejected before the Welford update.
*/
func (metric *Metric) finalize(n float64) error {
	if err := errnie.Require(map[string]any{
		"raw": metric.Raw,
	}); err != nil {
		return errnie.Error(err)
	}

	delta := metric.Raw - metric.center
	metric.center += delta / n

	if variance := (metric.scale*metric.scale*(n-core.Unit) + delta*(metric.Raw-metric.center)) / n; variance > 0 {
		metric.scale = math.Sqrt(variance)
		metric.Standardized = (metric.Raw - metric.center) / metric.scale
		metric.Normalized = math.Tanh(metric.Standardized)
	}

	return metric.valid()
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

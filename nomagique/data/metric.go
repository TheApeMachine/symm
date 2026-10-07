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
		unit:      unit,
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
		unit:      unit,
		timescale: timescale,
	}
}

func (metric *Metric) Unit() Unit {
	if metric == nil {
		return ""
	}

	return metric.unit
}

func (metric *Metric) Timescale() Timescale {
	if metric == nil {
		return ""
	}

	return metric.timescale
}

/*
finalize is called from the Measurement to set the derived values, like
center, scale, normalized, and standardized values. An invalid observation
is rejected before the Welford update.
*/
func (metric *Metric) finalize(n float64) error {
	// The observation is checked before it touches the Welford state: a
	// producer that computed an undefined Raw fails this Measurement here,
	// and the center/scale it would have poisoned stay as they were.
	if err := metric.valid("label", "raw", "unit", "timescale"); err != nil {
		return err
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
func (metric *Metric) valid(fields ...string) error {
	if len(fields) > 0 {
		mapped := make(map[string]any)

		for _, field := range fields {
			switch field {
			case "label":
				mapped[field] = metric.Label
			case "raw":
				mapped[field] = metric.Raw
			case "normalized":
				mapped[field] = metric.Normalized
			case "standardized":
				mapped[field] = metric.Standardized
			case "exact":
				mapped[field] = metric.Exact
			case "center":
				mapped[field] = metric.center
			case "scale":
				mapped[field] = metric.scale
			case "unit":
				mapped[field] = metric.unit
			case "timescale":
				mapped[field] = metric.timescale
			case "updated":
				mapped[field] = true
			}
		}

		return errnie.Error(
			errnie.Require(mapped),
			"metric", metric.Label,
			"raw", metric.Raw,
			"center", metric.center,
			"scale", metric.scale,
		)
	}

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

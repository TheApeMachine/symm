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
	label        string           // The name of the metric.
	raw          float64          // The raw value of the metric.
	normalized   float64          // The normalized value of the metric.
	standardized float64          // The standardized value of the metric.
	exact        *decimal.Decimal // The exact value of the metric.
	center       float64          // The center of the metric.
	scale        float64          // The scale of the metric.
	unit         Unit             // The unit of the metric.
	timescale    Timescale        // The timescale of the metric.
}

/*
NewMetric builds a metric declaring its label, unit, timescale, and the
center and scale its values are standardized against.
*/
func NewMetric(
	label string, raw float64, unit Unit, timescale Timescale,
) Metric {
	return Metric{
		label:     label,
		raw:       raw,
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
) Metric {
	return Metric{
		label:     label,
		raw:       exact.Float64(),
		exact:     exact,
		unit:      unit,
		timescale: timescale,
	}
}

/*
finalize is called from the Measurement to set the derived values, like
center, scale, normalized, and standardized values.
*/
func (metric *Metric) finalize(n float64) error {
	delta := metric.raw - metric.center
	metric.center += delta / n

	if variance := (metric.scale*metric.scale*(n-core.Unit) + delta*(metric.raw-metric.center)) / n; variance > 0 {
		metric.scale = math.Sqrt(variance)
		metric.standardized = (metric.raw - metric.center) / metric.scale
		metric.normalized = math.Tanh(metric.standardized)
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
				mapped[field] = metric.label
			case "raw":
				mapped[field] = metric.raw
			case "normalized":
				mapped[field] = metric.normalized
			case "standardized":
				mapped[field] = metric.standardized
			case "exact":
				mapped[field] = metric.exact
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

		return errnie.Error(errnie.Require(mapped))
	}

	return errnie.Error(errnie.Require(map[string]any{
		"label":        metric.label,
		"raw":          metric.raw,
		"normalized":   metric.normalized,
		"standardized": metric.standardized,
		"exact":        metric.exact,
		"center":       metric.center,
		"scale":        metric.scale,
		"unit":         metric.unit,
		"timescale":    metric.timescale,
		"updated":      true,
	}))
}

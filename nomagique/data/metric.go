package data

import (
	"math"
	"strings"

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
	support      float64          // The prior observations Standardized was ranked against.
	Raw          float64          // The raw value of the metric.
	Normalized   float64          // The normalized value of the metric.
	Standardized float64          // The standardized value of the metric.
	Exact        *decimal.Decimal // The exact value of the metric.
	unit         Unit             // The unit of the metric.
	timescale    Timescale        // The timescale of the metric.
}

/*
NewMetric builds a metric declaring its label, raw value, unit, and timescale.
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

func (metric *Metric) CleanLabel() string {
	base := metric.Label

	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	return base
}

func (metric *Metric) Unit() Unit {
	return metric.unit
}

func (metric *Metric) Timescale() Timescale {
	return metric.timescale
}

func (metric *Metric) Support() float64 {
	return metric.support
}

/*
finalize is called from the Measurement to standardize the observation against
its stream: Standardized is the standard normal quantile of Raw's mid-rank
among the stream's earlier observations, and support is how many there were.
While the stream has too few of them the z-score is undefined: Standardized,
Normalized, and support stay zero and Standardizable reports false. An invalid
observation is rejected before it reaches the stream.
*/
func (metric *Metric) finalize(state *standardizer) error {
	if err := errnie.Require(map[string]any{
		"raw": metric.Raw,
	}); err != nil {
		return errnie.Error(err)
	}

	standardized, support, defined := state.observe(metric.Raw)
	metric.support = support
	metric.Standardized = 0
	metric.Normalized = 0

	if defined {
		metric.Standardized = standardized
		metric.Normalized = math.Tanh(standardized)
	}

	return metric.valid()
}

/*
Standardizable reports whether Standardized holds a z-score. It is false until
the metric's stream has enough prior observations to rank against; an
undefined z-score is missing evidence, not a zero deformation.
*/
func (metric *Metric) Standardizable() bool {
	return metric.support > 0
}

/*
valid checks if the Measurement is valid.
*/
func (metric *Metric) valid() error {
	if metric.Label == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation, "label is required", nil,
		))
	}

	if metric.unit == "" || metric.unit == UnitNone {
		return errnie.Error(errnie.Err(
			errnie.Validation, "unit is required", nil,
		))
	}

	if metric.timescale == "" || metric.timescale == TimescaleNone {
		return errnie.Error(errnie.Err(
			errnie.Validation, "timescale is required", nil,
		))
	}

	if math.IsNaN(metric.Raw) || math.IsInf(metric.Raw, 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation, "raw is not a finite number", nil,
		))
	}

	return nil
}

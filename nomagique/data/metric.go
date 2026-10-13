package data

import (
	"math"
	"math/big"
	"strings"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type rawDecimal struct {
	integer   *big.Int
	increment int64
	scale     int64
}

var pow10 = [...]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18,
}

func decimalFloat64(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}

	raw := (*rawDecimal)(unsafe.Pointer(d))

	if raw.integer == nil {
		return 0
	}

	scale := raw.scale

	if scale >= 0 && scale <= 18 && raw.integer.IsInt64() {
		return float64(raw.integer.Int64()) / pow10[scale]
	}

	value, _ := d.Rat().Float64()
	return value
}

/*
Metric is one projected value of a measurement: a label, the raw observation,
its normalized and standardized forms, and the physical unit and timescale.

Exact retains the venue's original decimal for observations the venue printed
exactly (prices, sizes). Ingest writes it and capture/audit reads it; the
mathematics runs on Raw alone and never consults it. Derived facts leave it
nil.
*/
type Metric struct {
	err       error
	label     string           // The name of the metric.
	snm       float64          // The self-normalized movement of the metric.
	raw       float64          // The raw value of the metric.
	exact     *decimal.Decimal // The exact value of the metric.
	unit      Unit             // The unit of the metric.
	timescale Timescale        // The timescale of the metric.
}

/*
NewMetric builds a metric declaring its label, raw value, unit, and timescale.
*/
func NewMetric(
	label string, raw float64, unit Unit, timescale Timescale,
) *Metric {
	return &Metric{
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
) *Metric {
	return &Metric{
		label:     label,
		raw:       decimalFloat64(exact),
		exact:     exact,
		unit:      unit,
		timescale: timescale,
	}
}

func (metric *Metric) Label() string {
	base := metric.label

	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	return base
}

func (metric *Metric) Raw() float64 {
	return metric.raw
}

func (metric *Metric) SNM() float64 {
	return metric.snm
}

func (metric *Metric) Exact() *decimal.Decimal {
	return metric.exact
}

func (metric *Metric) Unit() Unit {
	return metric.unit
}

func (metric *Metric) Timescale() Timescale {
	return metric.timescale
}

/*
finalize is called from the Measurement to validate the observation before
locking.
*/
func (metric *Metric) finalize() error {
	return metric.valid()
}

/*
setSNM (Self-Normalized Movement) takes the reference from the prior magnitude
and the movement itself. It measures a transition, not an isolated value.
The result stays within \([-1,1]\) without statistical calibration.
*/
func (metric *Metric) setSNM(prior float64) {
	delta := metric.raw - prior
	denominator := math.Abs(prior) + math.Abs(delta)

	if denominator == 0 {
		metric.snm = 0
		return
	}

	metric.snm = delta / denominator
}

/*
valid checks if the Measurement is valid.
*/
func (metric *Metric) valid() error {
	if metric.label == "" {
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

	if math.IsNaN(metric.raw) || math.IsInf(metric.raw, 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation, "raw is not a finite number", nil,
		))
	}

	return nil
}

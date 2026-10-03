package data

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Metric is one projected value of a measurement: a label, the raw observation,
its normalized and standardized forms, and the physical unit and timescale.

Exact retains the venue's original decimal for observations the venue printed
exactly (prices, sizes). Ingest writes it and capture/audit reads it; the
mathematics runs on Raw alone and never consults it. Derived facts leave it
nil.
*/
type Metric[Value any] struct {
	Label        string           `json:"label"`
	Raw          Value            `json:"raw"`
	Normalized   *Value           `json:"normalized,omitempty"`
	Standardized *Value           `json:"standardized,omitempty"`
	Deformation  *Value           `json:"deformation,omitempty"`
	Exact        *decimal.Decimal `json:"exact,omitempty"`
	Center       float64          `json:"center,omitempty"`
	Scale        float64          `json:"scale,omitempty"`
	Unit         Unit             `json:"unit,omitempty"`
	Timescale    Timescale        `json:"timescale,omitempty"`
	X            int64            `json:"x"`
	Y            int64            `json:"y"`
	Region       uint8            `json:"region"`
}

/*
NewMetric builds a metric declaring its label, unit, timescale, and the
center and scale its values are standardized against.
*/
func NewMetric[Value any](
	label string, unit Unit, timescale Timescale, center, scale float64,
) Metric[Value] {
	return Metric[Value]{
		Label:     label,
		Center:    center,
		Scale:     scale,
		Unit:      unit,
		Timescale: timescale,
	}
}

/*
Write sets the metric's raw value and its standardized form against the
center and scale declared at registration.
*/
func (metric Metric[T]) Write(value T) Metric[T] {
	metric.Raw = value

	if number, held := any(value).(float64); held {

		if metric.Scale != 0 {
			standard := (number - metric.Center) / metric.Scale
			metric.Standardized = any(&standard).(*T)
		}
	}

	return metric
}

/*
Deformation is the dimensionless, signed amount by which a metric has changed
relative to its immediately previous observation. It is derived from the
relative change in absolute magnitude, with the direction taken from the actual
movement. Proportional moves are scale-free, unchanged values produce 0, and
reversals of equal magnitude produce 0.
*/
func Deformation(previous, current float64) float64 {
	if current == previous {
		return 0
	}

	previousMagnitude := math.Abs(previous)
	currentMagnitude := math.Abs(current)
	extent := previousMagnitude + currentMagnitude

	if extent == 0 {
		return 0
	}

	magnitudeMovement := math.Abs(currentMagnitude-previousMagnitude) / extent

	if magnitudeMovement == 0 {
		return 0
	}

	if current > previous {
		return magnitudeMovement
	}

	return -magnitudeMovement
}

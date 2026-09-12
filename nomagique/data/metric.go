package data

import (
	"unsafe"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

var standardizer = statistic.NewStandardize()

/*
Metric is one projected value of a measurement: a label, the raw observation,
its normalized and standardized forms, and the physical unit and timescale.
*/
type Metric[Value any] struct {
	Label        string      `json:"label"`
	Raw          Value       `json:"raw"`
	Normalized   *Value      `json:"normalized,omitempty"`
	Standardized *Value      `json:"standardized,omitempty"`
	Center       float64     `json:"center,omitempty"`
	Scale        float64     `json:"scale,omitempty"`
	Unit         Unit        `json:"unit,omitempty"`
	Timescale    Timescale   `json:"timescale,omitempty"`
	Coordinates  *[2]float64 `json:"-"`
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
		input := statistic.StandardizeInput{Value: number, Center: metric.Center, Scale: metric.Scale}

		for out := range standardizer.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
			standard := *(*float64)(out)
			metric.Standardized = any(&standard).(*T)
		}
	}

	return metric
}

package data

/*
Metric is one projected value of a measurement: a label, the raw observation,
its normalized and standardized forms, and the physical unit and timescale.
*/
type Metric[Value any] struct {
	Label        string    `json:"label"`
	Raw          Value     `json:"raw"`
	Normalized   *Value    `json:"normalized,omitempty"`
	Standardized *Value    `json:"standardized,omitempty"`
	Unit         Unit      `json:"unit,omitempty"`
	Timescale    Timescale `json:"timescale,omitempty"`
	// Coordinates is the live position owned by the numerical grid. A nil
	// pointer means this metric has not been admitted to a grid.
	Coordinates *[2]float64 `json:"-"`
}

/*
NewMetric builds a metric with raw, normalized, and standardized forms.
*/
func NewMetric[Value any](
	label string,
	raw Value,
	normalized *Value,
	standardized *Value,
	unit Unit,
	timescale Timescale,
) Metric[Value] {
	return Metric[Value]{
		Label:        label,
		Raw:          raw,
		Normalized:   normalized,
		Standardized: standardized,
		Unit:         unit,
		Timescale:    timescale,
	}
}

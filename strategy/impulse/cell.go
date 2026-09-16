package impulse

import (
	"math"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Cell addresses a metric at its producer. It owns only its coordinate and
movement statistics; the metric's value is never stored in the map.
*/
type Cell struct {
	ID       uint64
	Owner    string
	Metric   string
	Position geometry.Point
	Baseline statistic.Moments
	Previous float64
	Movement float64
	Level    float64
	Activity float64
	Present  bool
	observed bool
	owner    *owner
}

type owner struct {
	measurement *data.Measurement[float64]
	sequence    int64
}

func (cell *Cell) Value() (float64, bool) {
	metric, exists := cell.owner.measurement.Metrics[cell.Metric]
	return metric.Raw, exists
}

/*
Observe reads the producer once. Symmetric relative change is dimensionless,
defined at a zero crossing, and independent of the metric's units. Previous is
the one-lag sufficient state required to measure movement, not retained tape.
*/
func (cell *Cell) Observe(sequence int64) {
	cell.Movement, cell.Activity = 0, 0
	cell.Present = cell.owner.measurement.SeqIdx == sequence && cell.owner.measurement.Err == nil

	if !cell.Present {
		cell.Position.Authority = 0
		return
	}

	value, present := cell.Value()
	cell.Present = present

	if !present {
		return
	}

	denominator := math.Abs(value) + math.Abs(cell.Previous)

	if cell.observed && denominator > 0 {
		cell.Movement = (value - cell.Previous) / denominator
	}

	cell.Level = 0

	if cell.Baseline.Count > 1 && cell.Baseline.M2 > 0 {
		cell.Level = (value - cell.Baseline.Mean) / math.Sqrt(cell.Baseline.M2/(cell.Baseline.Count-1))
	}

	cell.Baseline.Update(value)
	cell.Previous, cell.observed = value, true
	measurement := cell.owner.measurement
	cell.Position.Authority = measurement.Maturity

	if measurement.Estimated && !measurement.SNRDefined {
		cell.Position.Authority = 0
	}

	if measurement.SNRDefined {
		cell.Position.Authority *= measurement.SNR / (1 + measurement.SNR)
	}

	cell.Activity = cell.Movement * cell.Movement * cell.Position.Authority
}

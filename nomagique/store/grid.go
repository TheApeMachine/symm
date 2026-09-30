package store

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Grid arranges metrics into a 2D coordinate space where metrics cluster sympathetically
according to co-movement, relative magnitude, and SNR authority.
*/
type Grid struct {
	*core.PrimitiveError
	Metrics []*data.Metric[float64] `json:"metrics"`
	Settled bool                    `json:"settled"`
	deltas  []float64
}

func NewGrid() *Grid {
	return &Grid{
		PrimitiveError: core.NewPrimitiveError(),
		Metrics:        make([]*data.Metric[float64], 0),
		deltas:         make([]float64, 0, 64),
	}
}

func (grid *Grid) Add(metric *data.Metric[float64]) {
	if metric == nil {
		return
	}

	if existing := grid.find(metric.Label); existing != nil {
		*existing = *metric
		return
	}

	grid.Metrics = append(grid.Metrics, metric)
}

func (grid *Grid) find(label string) *data.Metric[float64] {
	for _, metric := range grid.Metrics {
		if metric.Label == label {
			return metric
		}
	}

	return nil
}

func (grid *Grid) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			measurement := (*data.Measurement[float64])(arriving)

			if measurement == nil || measurement.Metrics == nil || len(measurement.Metrics) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			grid.Update(measurement)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (grid *Grid) Update(measurement *data.Measurement[float64]) {
	grid.update(measurement, true)
}

func (grid *Grid) Observe(measurement *data.Measurement[float64]) {
	grid.update(measurement, false)
}

func (grid *Grid) Region(label string) uint8 {
	stored := grid.find(label)

	if stored != nil {
		return stored.Region
	}

	return 0
}

func (grid *Grid) update(measurement *data.Measurement[float64], decorate bool) {
	if grid.Settled {
		if decorate {
			for label, incoming := range measurement.Metrics {
				stored := grid.find(label)

				if stored != nil {
					incoming.X = stored.X
					incoming.Y = stored.Y
					incoming.Region = stored.Region
					measurement.Metrics[label] = incoming
				}
			}
		}

		return
	}

	for label, incoming := range measurement.Metrics {
		existing := grid.find(label)

		if existing == nil {
			metricCount := int64(len(grid.Metrics))
			width := int64(math.Ceil(math.Sqrt(float64(metricCount + 1))))

			if width < 1 {
				width = 1
			}

			created := incoming
			created.X = metricCount % width
			created.Y = metricCount / width
			grid.Metrics = append(grid.Metrics, &created)
		}
	}

	metricCount := len(grid.Metrics)

	if metricCount <= 1 {
		if decorate {
			for label, incoming := range measurement.Metrics {
				stored := grid.find(label)

				if stored != nil {
					incoming.X = stored.X
					incoming.Y = stored.Y
					incoming.Region = 1
					measurement.Metrics[label] = incoming
				}
			}
		}

		return
	}

	width := max(int64(math.Ceil(math.Sqrt(float64(metricCount)))), 1)
	maxBound := max(width*2, 32)

	if cap(grid.deltas) < metricCount {
		grid.deltas = make([]float64, metricCount)
	}

	if cap(grid.deltas) >= metricCount {
		grid.deltas = grid.deltas[:metricCount]
		clear(grid.deltas)
	}

	deltas := grid.deltas

	for index, metric := range grid.Metrics {
		if incoming, found := measurement.Metrics[metric.Label]; found {
			deltas[index] = incoming.Raw - metric.Raw
			metric.Raw = incoming.Raw
		}
	}

	moved := false

	clamp := func(val, minVal, maxVal int64) int64 {
		if val < minVal {
			return minVal
		}

		if val > maxVal {
			return maxVal
		}

		return val
	}

	// Apply sympathetic attraction/repulsion rules directly on Metric.X and Metric.Y
	for firstIdx := 0; firstIdx < metricCount; firstIdx++ {
		firstMetric := grid.Metrics[firstIdx]
		firstDelta := deltas[firstIdx]

		for secondIdx := firstIdx + 1; secondIdx < metricCount; secondIdx++ {
			secondMetric := grid.Metrics[secondIdx]
			secondDelta := deltas[secondIdx]

			attraction := -1.0

			if (firstDelta >= 0 && secondDelta >= 0) || (firstDelta <= 0 && secondDelta <= 0) {
				attraction = 1.0
			}

			diffX := secondMetric.X - firstMetric.X
			diffY := secondMetric.Y - firstMetric.Y

			if diffX == 0 && diffY == 0 && attraction > 0 {
				continue
			}

			if attraction > 0 {
				if diffX == 1 {
					firstMetric.X = secondMetric.X
					moved = true
				}

				if diffX == -1 {
					secondMetric.X = firstMetric.X
					moved = true
				}

				if diffY == 1 {
					firstMetric.Y = secondMetric.Y
					moved = true
				}

				if diffY == -1 {
					secondMetric.Y = firstMetric.Y
					moved = true
				}

				if math.Abs(float64(diffX)) > 1 {
					stepX := int64(math.Copysign(1, float64(diffX)))
					new1X := clamp(firstMetric.X+stepX, 0, maxBound)
					new2X := clamp(secondMetric.X-stepX, 0, maxBound)

					if new1X != firstMetric.X || new2X != secondMetric.X {
						firstMetric.X = new1X
						secondMetric.X = new2X
						moved = true
					}
				}

				if math.Abs(float64(diffY)) > 1 {
					stepY := int64(math.Copysign(1, float64(diffY)))
					new1Y := clamp(firstMetric.Y+stepY, 0, maxBound)
					new2Y := clamp(secondMetric.Y-stepY, 0, maxBound)

					if new1Y != firstMetric.Y || new2Y != secondMetric.Y {
						firstMetric.Y = new1Y
						secondMetric.Y = new2Y
						moved = true
					}
				}

				continue
			}

			// Repulsion pushes metrics apart only within the local interaction neighborhood
			if math.Abs(float64(diffX)) <= float64(width) && math.Abs(float64(diffY)) <= float64(width) {
				stepX := int64(math.Copysign(1, float64(diffX)))
				stepY := int64(math.Copysign(1, float64(diffY)))

				if diffX == 0 && diffY == 0 {
					stepX = 1
				}

				new1X := clamp(firstMetric.X-stepX, 0, maxBound)
				new1Y := clamp(firstMetric.Y-stepY, 0, maxBound)
				new2X := clamp(secondMetric.X+stepX, 0, maxBound)
				new2Y := clamp(secondMetric.Y+stepY, 0, maxBound)

				if new1X != firstMetric.X || new1Y != firstMetric.Y || new2X != secondMetric.X || new2Y != secondMetric.Y {
					firstMetric.X = new1X
					firstMetric.Y = new1Y
					secondMetric.X = new2X
					secondMetric.Y = new2Y
					moved = true
				}
			}
		}
	}

	// Update regions directly on metrics
	for _, metric := range grid.Metrics {
		regionX := clamp(metric.X, 0, maxBound)
		regionY := clamp(metric.Y, 0, maxBound)
		metric.X = regionX
		metric.Y = regionY
		metric.Region = uint8((regionY*width+regionX)%255 + 1)
	}

	// Decorate incoming measurement
	if decorate {
		for label, incoming := range measurement.Metrics {
			stored := grid.find(label)

			if stored != nil {
				incoming.X = stored.X
				incoming.Y = stored.Y
				incoming.Region = stored.Region
				measurement.Metrics[label] = incoming
			}
		}
	}

	if !moved {
		grid.Settled = true
	}
}

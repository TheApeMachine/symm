package store

import (
	"iter"
	"math"
	"slices"
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
	Metrics      []*data.Metric[float64] `json:"metrics"`
	Settled      bool                    `json:"settled"`
	settledTicks int
	deltas       []float64
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

/*
LitRegions computes the total activation of each region based on incoming measurement
metrics and returns the top N most active region identifiers as a token slice (e.g. [A, B, C]).
*/
func (grid *Grid) LitRegions(measurement *data.Measurement[float64], topN int) []byte {
	if measurement == nil || len(measurement.Metrics) == 0 || topN <= 0 {
		return nil
	}

	var activity [256]float64
	var present [256]bool

	weight := 1.0
	if measurement.SNRDefined && measurement.SNR > 0 {
		weight = measurement.SNR
	}

	for label, metric := range measurement.Metrics {
		region := metric.Region
		stored := grid.find(label)

		if stored != nil && stored.Region > 0 {
			region = stored.Region
		}

		if region > 0 {
			activity[region] += math.Abs(metric.Raw) * weight
			present[region] = true
		}
	}

	type regionScore struct {
		id    uint8
		score float64
	}

	scores := make([]regionScore, 0, 16)
	for i := 1; i < 256; i++ {
		if present[i] && activity[i] > 0 {
			scores = append(scores, regionScore{id: uint8(i), score: activity[i]})
		}
	}

	slices.SortFunc(scores, func(a, b regionScore) int {
		if b.score > a.score {
			return 1
		}
		if b.score < a.score {
			return -1
		}
		return int(a.id) - int(b.id)
	})

	limit := min(topN, len(scores))
	if limit == 0 {
		return nil
	}

	token := make([]byte, limit)
	for i := 0; i < limit; i++ {
		token[i] = scores[i].id
	}

	return token
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

	addedNew := false

	for _, incoming := range measurement.Metrics {
		existing := grid.find(incoming.Label)

		if existing == nil {
			created := incoming
			grid.Metrics = append(grid.Metrics, &created)
			addedNew = true
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

	if addedNew {
		for idx, metric := range grid.Metrics {
			if metric.X == 0 && metric.Y == 0 && idx > 0 {
				metric.X = int64(idx) % width
				metric.Y = int64(idx) / width
			}
		}
	}

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

			diffX := secondMetric.X - firstMetric.X
			diffY := secondMetric.Y - firstMetric.Y

			// Exclusion: two metrics cannot occupy the exact same coordinate
			if diffX == 0 && diffY == 0 {
				new1X := clamp(firstMetric.X-1, 0, maxBound)
				new2X := clamp(secondMetric.X+1, 0, maxBound)

				if new1X != firstMetric.X || new2X != secondMetric.X {
					firstMetric.X = new1X
					secondMetric.X = new2X
					moved = true
				}

				continue
			}

			// Co-movement requires active, non-zero deltas in both metrics
			if firstDelta == 0 || secondDelta == 0 {
				continue
			}

			attraction := -1.0

			if (firstDelta > 0 && secondDelta > 0) || (firstDelta < 0 && secondDelta < 0) {
				attraction = 1.0
			}

			if attraction > 0 {
				// Attract: pull metrics closer along axes where distance > 1
				// Do not collapse neighboring cells onto the same coordinate
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

			// Repulsion: counter-movement pushes metrics apart within local neighborhood
			if math.Abs(float64(diffX)) <= float64(width) && math.Abs(float64(diffY)) <= float64(width) {
				stepX := int64(math.Copysign(1, float64(diffX)))
				stepY := int64(math.Copysign(1, float64(diffY)))

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

	// Update regions directly on metrics and measure regional diversity
	regionSet := make(map[uint8]struct{}, metricCount)

	for _, metric := range grid.Metrics {
		regionX := clamp(metric.X, 0, maxBound)
		regionY := clamp(metric.Y, 0, maxBound)
		metric.X = regionX
		metric.Y = regionY
		metric.Region = uint8((regionY*width+regionX)%255 + 1)
		regionSet[metric.Region] = struct{}{}
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

	// Settling requires stability across observations and genuine regional diversity
	if moved {
		grid.settledTicks = 0
	}

	if !moved && metricCount >= 2 && len(regionSet) > 1 {
		grid.settledTicks++

		if grid.settledTicks >= 10 {
			grid.Settled = true
		}
	}
}

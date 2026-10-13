package store

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Grid is not a grid at all, it is a "group by" over Measurement
and Metric pins into 12 multi-signal confluence regions.
*/
type Grid struct {
	*runtime.System
}

/*
RegionCount is the number of confluence regions the pins map metrics into.
*/
const RegionCount = 12

func NewGrid(ctx context.Context) *Grid {
	return &Grid{
		System: runtime.NewSystem(ctx, "grid"),
	}
}

/*
Region is one confluence region's evidence in a frame: Deformation is the sum
of its self-normalized metric magnitudes |SNM|, Counts how many metrics it holds,
and Brightness their average in [0, 1].
*/
type Region struct {
	Key         uint8
	Deformation float64
	Brightness  float64
	Counts      int
}

/*
Observe pins every metric of the frame's peers to its region and returns the
12 regions sorted brightest first. A region's brightness is the average of its
metrics' |SNM| in [0, 1]. Only the magnitude matters, never the direction, and
an average does not grow with the number of metrics, so a large region has no
edge over a small one.

Observe returns nil when a metric is not pinned to a region (a contract breach,
reported once as an error, after which the grid observes nothing), or when a
region holds no metric at all.
*/
func (grid *Grid) Observe(measurement *data.Measurement) []*Region {
	if grid.Error() != nil {
		return nil
	}

	regions := make([]*Region, RegionCount)

	for idx := range regions {
		regions[idx] = &Region{Key: uint8(idx)}
	}

	for _, peer := range measurement.Peers() {
		surprisePerMetric := 0.0

		if metricCount := peer.MetricCount(); metricCount > 0 {
			surprisePerMetric = peer.Surprise() / float64(metricCount)
		}

		excitation := 1.0 + surprisePerMetric

		for entry := range peer.Read() {
			region, isPinned := regionMetricPins[peer.Source][entry.Metric.Label()]

			if !isPinned {
				grid.Error(errnie.Err(
					errnie.NotAcceptable,
					fmt.Sprintf("[grid] metric not pinned to a region: %s/%s",
						peer.Source, entry.Metric.Label(),
					),
					nil,
				))

				return nil
			}

			regions[region].Deformation += math.Abs(entry.Metric.SNM()) * excitation
			regions[region].Counts++
		}
	}

	for _, region := range regions {
		if region.Counts == 0 {
			return nil
		}

		region.Brightness = region.Deformation / float64(region.Counts)
	}

	slices.SortFunc(regions, func(first, second *Region) int {
		if first.Brightness > second.Brightness {
			return -1
		}

		if first.Brightness < second.Brightness {
			return 1
		}

		return 0
	})

	return regions
}

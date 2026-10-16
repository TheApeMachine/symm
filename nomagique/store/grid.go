package store

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Grid is not a grid at all, it is a "group by" over Measurement
and Metric pins into 15 multi-signal confluence regions.
*/
type Grid struct {
	*runtime.System
}

/*
RegionCount is the number of confluence regions the pins map metrics into.
*/
const RegionCount = 15

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
Observe pins every metric of the frame to its region and returns the
15 regions sorted brightest first.
*/
func (grid *Grid) Observe(value *data.Value) []*Region {
	if grid.Error() != nil || value == nil {
		return nil
	}

	regions := make([]*Region, RegionCount)
	for idx := range regions {
		regions[idx] = &Region{Key: uint8(idx)}
	}

	totalCounts := 0

	for indexKey, floats := range data.Indexed[[]float64](value, types.SignalSourceStrings...) {
		parts := strings.Split(indexKey, "/")
		if len(parts) == 0 {
			continue
		}

		source := parts[0]
		sourcePins, hasSource := regionMetricPins[source]
		if !hasSource {
			continue
		}

		offset := max(len(parts)-len(floats), 0)
		for idx, floatVal := range floats {
			if offset+idx >= len(parts) {
				break
			}

			label := parts[offset+idx]
			targetRegions, isPinned := sourcePins[label]
			if !isPinned {
				continue
			}

			for _, region := range targetRegions {
				regions[region].Deformation += math.Abs(floatVal)
				regions[region].Counts++
				totalCounts++
			}
		}
	}

	if totalCounts == 0 {
		return nil
	}

	for _, region := range regions {
		if region.Counts > 0 {
			region.Brightness = region.Deformation / float64(region.Counts)
		}
	}

	slices.SortFunc(regions, func(first, second *Region) int {
		if first.Brightness > second.Brightness {
			return -1
		}
		if first.Brightness < second.Brightness {
			return 1
		}
		return int(first.Key) - int(second.Key)
	})

	return regions
}

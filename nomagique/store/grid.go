package store

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"iter"
	"math"
	"slices"
	"sync/atomic"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type Grid struct {
	*core.PrimitiveError
	Metrics     []*data.Metric[float64]
	HYEstimator core.Primitive
	
	settled bool
	updates atomic.Int64
}

func NewGrid() *Grid {
	return &Grid{
		PrimitiveError: core.NewPrimitiveError(),
		HYEstimator:    algo.NewHayashiYoshida(),
	}
}

/*
Update assigns spatial coordinates (X, Y) and topological sympathy regions (1..20)
to the metrics in measurement and all associated peers, so the Learning Dashboard's
Impulse Map has rich, active spatial topography.
*/
func (sg *Grid) Update(measurement *data.Measurement[float64]) {
	if measurement == nil {
		return
	}

	sg.updates.Add(1)
	assignMetricRegions(measurement, 0)
}

func assignMetricRegions(measurement *data.Measurement[float64], depth int) {
	if measurement == nil || depth > 2 {
		return
	}

	for i := range measurement.Metrics {
		metric := &measurement.Metrics[i].Metric
		key := measurement.Metrics[i].Key
		if key == "" {
			key = metric.Label
		}

		h := fnv.New32a()
		h.Write([]byte(key))

		// Map into 20 regions (1-indexed for the UI)
		regionID := (h.Sum32() % 20) + 1
		metric.Region = uint8(regionID)

		// Assign deterministic X, Y for the Topography map visualization (5x4 grid)
		metric.X = int64((regionID - 1) % 5) * 200
		metric.Y = int64((regionID - 1) / 5) * 200
	}

	for _, peer := range measurement.Peers {
		if peer != nil {
			assignMetricRegions(peer, depth+1)
		}
	}
}

func (sg *Grid) IsSettled() bool {
	// Simulate grid development stage for the first 50 ticks
	if sg.settled {
		return true
	}

	if sg.updates.Load() > 50 {
		return true
	}

	return false
}

func (sg *Grid) Settle() {
	sg.settled = true
}

const litRegionTokenSize = 3

type regionScore struct {
	region uint8
	value  float64
}

/*
LitRegions buckets metrics into exactly 20 static regions based on a deterministic
hash of their keys. The top N most lit regions by cumulative activity score
(Standardized/Normalized, or Raw presence) are selected and deterministically
sorted (ascending by region ID) to produce the canonical precursor token signature (TRAINING.md N=3).
*/
func (sg *Grid) LitRegions(measurements ...*data.Measurement[float64]) [][]byte {
	activity := make(map[uint8]float64)

	for _, m := range measurements {
		if m == nil {
			continue
		}

		// Ensure regions are assigned even on historical precursor scans
		sg.Update(m)
		collectRegionActivity(m, activity, 0)
	}

	scores := make([]regionScore, 0, len(activity))
	for r, act := range activity {
		if r > 0 && act > 0 {
			scores = append(scores, regionScore{region: r, value: act})
		}
	}

	if len(scores) == 0 {
		return nil
	}

	// Sort by activity descending, break ties with region ascending
	slices.SortFunc(scores, func(left, right regionScore) int {
		if left.value > right.value {
			return -1
		}
		if left.value < right.value {
			return 1
		}
		return cmp.Compare(left.region, right.region)
	})

	limit := min(litRegionTokenSize, len(scores))
	lit := scores[:limit]

	regionIDs := make([]int, 0, len(lit))
	for _, s := range lit {
		regionIDs = append(regionIDs, int(s.region))
	}
	slices.Sort(regionIDs)

	tokens := make([][]byte, 0, len(regionIDs))
	for _, id := range regionIDs {
		tokens = append(tokens, fmt.Appendf(nil, "R%d", id))
	}

	return tokens
}

func collectRegionActivity(measurement *data.Measurement[float64], activity map[uint8]float64, depth int) {
	if measurement == nil || depth > 2 {
		return
	}

	for _, entry := range measurement.Metrics {
		if entry.Metric.Region > 0 {
			act := regionActivity(entry.Metric)
			if act > 0 {
				activity[entry.Metric.Region] += act
			}
		}
	}

	for _, peer := range measurement.Peers {
		collectRegionActivity(peer, activity, depth+1)
	}
}

func regionActivity(metric data.Metric[float64]) float64 {
	if metric.Standardized != nil {
		return math.Abs(*metric.Standardized)
	}

	if metric.Normalized != nil {
		return math.Abs(*metric.Normalized)
	}

	if metric.Raw == 0 {
		return 0
	}

	return 1.0
}

func (sg *Grid) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {}
}

func (sg *Grid) Snapshot() ([]byte, error) {
	return sonic.Marshal(sg)
}

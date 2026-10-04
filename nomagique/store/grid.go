package store

import (
	"cmp"
	"fmt"
	"iter"
	"math"
	"slices"
	"sync/atomic"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type Grid struct {
	*core.PrimitiveError

	settled bool
	updates atomic.Int64
}

func NewGrid() *Grid {
	return &Grid{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Update assigns spatial coordinates (X, Y) and topological sympathy regions (1..20)
to the metrics in measurement and all associated peers, so the Learning Dashboard's
Impulse Map has rich, active spatial topography.
*/
func (sg *Grid) Update(measurement *data.Measurement[float64]) {
	if measurement == nil || sg.settled {
		return
	}

	sg.updates.Add(1)
	assignMetricRegions(measurement, 0)

	if sg.isMature(measurement) {
		sg.settled = true
	}
}

func fnv1a32(key string) uint32 {
	var hash uint32 = 2166136261
	for index := 0; index < len(key); index++ {
		hash ^= uint32(key[index])
		hash *= 16777619
	}
	return hash
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

		// Map into 20 regions (1-indexed for the UI)
		regionID := (fnv1a32(key) % 20) + 1
		metric.Region = uint8(regionID)

		// Assign deterministic X, Y for the Topography map visualization (5x4 grid)
		metric.X = int64((regionID-1)%5) * 200
		metric.Y = int64((regionID-1)/5) * 200
	}

	for _, peer := range measurement.Peers {
		if peer != nil {
			assignMetricRegions(peer, depth+1)
		}
	}
}

func (sg *Grid) isMature(measurement *data.Measurement[float64]) bool {
	if measurement == nil {
		return false
	}

	if len(measurement.Peers) == 0 {
		return measurement.Maturity >= 0.95
	}

	for _, peer := range measurement.Peers {
		if peer == nil || peer.Maturity < 0.95 {
			return false
		}
	}

	return true
}

func (sg *Grid) IsSettled() bool {
	return sg.settled
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

		assignMetricRegions(m, 0)
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

	quality := math.Max(0.0, math.Min(1.0, measurement.Maturity))

	facts := measurement.Facts()
	if facts.HasSupport {
		if facts.Support > 1 {
			supportDamp := 1.0 - (1.0 / facts.Support)
			quality = math.Min(quality, supportDamp)
		} else {
			quality *= 0.1
		}
	}

	if measurement.SNRDefined && measurement.SNR > 0 {
		quality *= measurement.SNR / (1.0 + measurement.SNR)
	}

	for _, entry := range measurement.Metrics {
		if entry.Metric.Region > 0 {
			activityScore := regionActivity(entry.Metric) * quality

			if activityScore > 0 {
				activity[entry.Metric.Region] += activityScore
			}
		}
	}

	for _, peer := range measurement.Peers {
		collectRegionActivity(peer, activity, depth+1)
	}
}

func regionActivity(metric data.Metric[float64]) float64 {
	if metric.Deformation == nil {
		return 0
	}

	return math.Abs(*metric.Deformation)
}

func (sg *Grid) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {}
}

func (sg *Grid) Snapshot() ([]byte, error) {
	return sonic.Marshal(sg)
}

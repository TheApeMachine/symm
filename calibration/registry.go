package calibration

import (
	"math"
	"sync/atomic"

	"github.com/theapemachine/symm/logic"
)

/*
CalibrationTarget records a settled forecast outcome for reliability tracking.
*/
type CalibrationTarget struct {
	Source           logic.SourceType
	Category         logic.CategoryType
	Horizon          float64
	PredictedMoveBps float64
	RealizedMoveBps  float64
	CostBps          float64
}

type bucketStats struct {
	count               int
	sumConfidence       float64
	sumEdgeBps          float64
	sumSurprise         float64
	sumSquaredEdge      float64
	sumPredictedMoveBps float64
	sumRealizedMoveBps  float64
	sumCostBps          float64
	positiveEdgeCount   int
	edgeSamples         []float64
}

type registrySnapshot struct {
	buckets map[string]bucketStats
}

func newRegistrySnapshot() *registrySnapshot {
	return &registrySnapshot{
		buckets: make(map[string]bucketStats),
	}
}

func cloneRegistrySnapshot(snapshot *registrySnapshot) *registrySnapshot {
	if snapshot == nil {
		return newRegistrySnapshot()
	}

	buckets := make(map[string]bucketStats, len(snapshot.buckets))

	for key, bucket := range snapshot.buckets {
		cloned := bucket
		cloned.edgeSamples = append([]float64(nil), bucket.edgeSamples...)
		buckets[key] = cloned
	}

	return &registrySnapshot{buckets: buckets}
}

/*
Registry accumulates per-source/category calibration buckets.
*/
type Registry struct {
	state atomic.Pointer[registrySnapshot]
}

func NewRegistry() *Registry {
	registry := &Registry{}
	registry.state.Store(newRegistrySnapshot())

	return registry
}

/*
Record appends a settled calibration target into reliability buckets.
*/
func (registry *Registry) Record(target CalibrationTarget, confidence float64) {
	if registry == nil {
		return
	}

	key := bucketKey(target.Source, target.Category)
	edgeBps := target.RealizedMoveBps - target.CostBps
	surprise := math.Abs(target.PredictedMoveBps - target.RealizedMoveBps)

	for {
		current := registry.state.Load()
		next := cloneRegistrySnapshot(current)
		bucket := next.buckets[key]

		bucket.count++
		bucket.sumConfidence += confidence
		bucket.sumEdgeBps += edgeBps
		bucket.sumSurprise += surprise
		bucket.sumSquaredEdge += edgeBps * edgeBps
		bucket.sumPredictedMoveBps += target.PredictedMoveBps
		bucket.sumRealizedMoveBps += target.RealizedMoveBps
		bucket.sumCostBps += target.CostBps
		bucket.edgeSamples = append(bucket.edgeSamples, edgeBps)

		if edgeBps > 0 {
			bucket.positiveEdgeCount++
		}

		next.buckets[key] = bucket

		if registry.state.CompareAndSwap(current, next) {
			return
		}
	}
}

/*
MeanEdgeByBucket returns average realized edge for a source/category bucket.
*/
func (registry *Registry) MeanEdgeByBucket(
	source logic.SourceType,
	category logic.CategoryType,
) (float64, bool) {
	bucket, ok := registry.Bucket(source, category)

	if !ok || bucket.Count == 0 {
		return 0, false
	}

	return bucket.MeanEdgeBps, true
}

func bucketKey(source logic.SourceType, category logic.CategoryType) string {
	return string(source) + ":" + string(category)
}

package calibration

import (
	"math"
	"sync"

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
	count          int
	sumConfidence  float64
	sumEdgeBps     float64
	sumSurprise    float64
	sumSquaredEdge float64
}

/*
Registry accumulates per-source/category calibration buckets.
*/
type Registry struct {
	mutex   sync.Mutex
	buckets map[string]*bucketStats
}

func NewRegistry() *Registry {
	return &Registry{
		buckets: make(map[string]*bucketStats),
	}
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

	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	bucket := registry.buckets[key]

	if bucket == nil {
		bucket = &bucketStats{}
		registry.buckets[key] = bucket
	}

	bucket.count++
	bucket.sumConfidence += confidence
	bucket.sumEdgeBps += edgeBps
	bucket.sumSurprise += surprise
	bucket.sumSquaredEdge += edgeBps * edgeBps
}

/*
MeanEdgeByBucket returns average realized edge for a source/category bucket.
*/
func (registry *Registry) MeanEdgeByBucket(
	source logic.SourceType,
	category logic.CategoryType,
) (float64, bool) {
	if registry == nil {
		return 0, false
	}

	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	bucket := registry.buckets[bucketKey(source, category)]

	if bucket == nil || bucket.count == 0 {
		return 0, false
	}

	return bucket.sumEdgeBps / float64(bucket.count), true
}

/*
EdgeConfidence returns a calibrated edge probability from historical bucket performance.
*/
func (registry *Registry) EdgeConfidence(
	source logic.SourceType,
	category logic.CategoryType,
	categoryConfidence float64,
) float64 {
	meanEdge, ok := registry.MeanEdgeByBucket(source, category)

	if !ok || meanEdge <= 0 {
		return categoryConfidence
	}

	calibrated := categoryConfidence * (1 - math.Exp(-meanEdge/25))

	if calibrated <= 0 {
		return categoryConfidence
	}

	if calibrated > 1 {
		return 1
	}

	return calibrated
}

func bucketKey(source logic.SourceType, category logic.CategoryType) string {
	return string(source) + ":" + string(category)
}

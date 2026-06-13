package calibration

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/symm/logic"
)

const MinCalibrationSamples = 30

const edgeConfidenceScaleBps = 25.0

/*
BucketSnapshot is a settled reliability view for one source/category pair.
*/
type BucketSnapshot struct {
	Source               logic.SourceType
	Category             logic.CategoryType
	Count                int
	MeanPredictedMoveBps float64
	MeanRealizedMoveBps  float64
	MeanCostBps          float64
	MeanEdgeBps          float64
	EdgeStdErr           float64
	HitRateAfterCost     float64
	Percentile10EdgeBps  float64
	Percentile50EdgeBps  float64
	Percentile90EdgeBps  float64
	CalibrationError     float64
	EligibleForSizing    bool
}

/*
Bucket returns the current snapshot for a source/category bucket.
*/
func (registry *Registry) Bucket(
	source logic.SourceType,
	category logic.CategoryType,
) (BucketSnapshot, bool) {
	if registry == nil {
		return BucketSnapshot{}, false
	}

	snapshot := registry.state.Load()
	bucket, ok := snapshot.buckets[bucketKey(source, category)]

	if !ok || bucket.count == 0 {
		return BucketSnapshot{}, false
	}

	count := float64(bucket.count)
	meanEdge := bucket.sumEdgeBps / count
	meanPredicted := bucket.sumPredictedMoveBps / count
	meanRealized := bucket.sumRealizedMoveBps / count
	meanCost := bucket.sumCostBps / count
	variance := bucket.sumSquaredEdge/count - meanEdge*meanEdge

	if variance < 0 {
		variance = 0
	}

	stdErr := math.Sqrt(variance / count)
	conservativeEdge := meanEdge - 1.65*stdErr
	hitRate := 0.0

	if bucket.count > 0 {
		hitRate = float64(bucket.positiveEdgeCount) / count
	}

	percentiles := bucket.edgePercentiles()
	calibrationError := math.Abs(meanPredicted-meanRealized) / math.Max(meanRealized, 1)

	return BucketSnapshot{
		Source:               source,
		Category:             category,
		Count:                bucket.count,
		MeanPredictedMoveBps: meanPredicted,
		MeanRealizedMoveBps:  meanRealized,
		MeanCostBps:          meanCost,
		MeanEdgeBps:          meanEdge,
		EdgeStdErr:           stdErr,
		HitRateAfterCost:     hitRate,
		Percentile10EdgeBps:  percentiles[0],
		Percentile50EdgeBps:  percentiles[1],
		Percentile90EdgeBps:  percentiles[2],
		CalibrationError:     calibrationError,
		EligibleForSizing:    bucket.count >= MinCalibrationSamples && conservativeEdge > 0,
	}, conservativeEdge > 0 || bucket.count >= MinCalibrationSamples
}

/*
EdgeConfidence returns conservative calibrated edge confidence for live risk control.
*/
func (registry *Registry) EdgeConfidence(
	source logic.SourceType,
	category logic.CategoryType,
	categoryConfidence float64,
) float64 {
	bucket, ok := registry.Bucket(source, category)

	if !ok || bucket.Count < MinCalibrationSamples {
		return 0
	}

	conservativeEdge := bucket.MeanEdgeBps - 1.65*bucket.EdgeStdErr

	if conservativeEdge <= 0 {
		return 0
	}

	return categoryConfidence * (1 - math.Exp(-conservativeEdge/edgeConfidenceScaleBps))
}

/*
ExpectedMoveBps returns calibrated expected move when enough settled observations exist.
*/
func (registry *Registry) ExpectedMoveBps(
	source logic.SourceType,
	category logic.CategoryType,
) (float64, bool) {
	bucket, ok := registry.Bucket(source, category)

	if !ok || !bucket.EligibleForSizing {
		return 0, false
	}

	if bucket.MeanRealizedMoveBps <= 0 {
		return 0, false
	}

	return bucket.MeanRealizedMoveBps, true
}

/*
SourceCategoryReports renders per-bucket calibration diagnostics.
*/
func (registry *Registry) SourceCategoryReports() []string {
	if registry == nil {
		return nil
	}

	snapshot := registry.state.Load()
	lines := make([]string, 0, len(snapshot.buckets))

	for key := range snapshot.buckets {
		parts := strings.SplitN(key, ":", 2)

		if len(parts) != 2 {
			continue
		}

		bucket, ok := registry.Bucket(
			logic.SourceType(parts[0]),
			logic.CategoryType(parts[1]),
		)

		if !ok {
			continue
		}

		lines = append(lines, formatBucketReport(bucket))
	}

	sort.Strings(lines)

	return lines
}

func formatBucketReport(bucket BucketSnapshot) string {
	return fmt.Sprintf(
		"source=%s category=%s samples=%d mean_expected_move_bps=%.1f mean_realized_move_bps=%.1f mean_cost_bps=%.1f mean_edge_bps=%.1f hit_rate_after_cost=%.2f p10_edge_bps=%.1f p50_edge_bps=%.1f p90_edge_bps=%.1f calibration_error=%.2f eligible_for_sizing=%t",
		bucket.Source,
		bucket.Category,
		bucket.Count,
		bucket.MeanPredictedMoveBps,
		bucket.MeanRealizedMoveBps,
		bucket.MeanCostBps,
		bucket.MeanEdgeBps,
		bucket.HitRateAfterCost,
		bucket.Percentile10EdgeBps,
		bucket.Percentile50EdgeBps,
		bucket.Percentile90EdgeBps,
		bucket.CalibrationError,
		bucket.EligibleForSizing,
	)
}

func (bucketStats *bucketStats) edgePercentiles() [3]float64 {
	if len(bucketStats.edgeSamples) == 0 {
		return [3]float64{}
	}

	sorted := append([]float64(nil), bucketStats.edgeSamples...)
	sort.Float64s(sorted)

	return [3]float64{
		percentile(sorted, 0.10),
		percentile(sorted, 0.50),
		percentile(sorted, 0.90),
	}
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	index := int(math.Round(quantile * float64(len(sorted)-1)))

	if index < 0 {
		index = 0
	}

	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

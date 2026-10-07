package audit

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAuditVitalityAndSympathy(t *testing.T) {
	Convey("Given simulated multi-metric observations", t, func() {
		ticks := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
		series := make(map[string]map[int64]float64)

		// Metric 1: Sine wave (healthy)
		// Metric 2: Highly correlated to Metric 1 (redundant clone)
		// Metric 3: Inverse sine wave (inverse sympathy)
		// Metric 4: Constant zero (dead)
		series["sensor_sin"] = make(map[int64]float64)
		series["sensor_clone"] = make(map[int64]float64)
		series["sensor_inv"] = make(map[int64]float64)
		series["sensor_dead"] = make(map[int64]float64)

		for _, tick := range ticks {
			val := math.Sin(float64(tick) * 0.5)
			series["sensor_sin"][tick] = val
			series["sensor_clone"][tick] = val + 0.001 // Collinear
			series["sensor_inv"][tick] = -val         // Inverse
			series["sensor_dead"][tick] = 0.0          // Dead
		}

		Convey("When analyzing metric vitality (Stage 1)", func() {
			vitality := AnalyzeVitality(ticks, series)

			So(vitality.TotalMetrics, ShouldEqual, 4)
			So(vitality.HealthyMetrics, ShouldEqual, 3)
			So(vitality.DeadMetrics, ShouldEqual, 1)
			So(len(vitality.RedundantPairs), ShouldBeGreaterThanOrEqualTo, 1)
			So(math.Abs(vitality.RedundantPairs[0].Correlation), ShouldBeGreaterThan, 0.99)
		})

		Convey("When analyzing pair sympathy against shuffled null (Stage 2)", func() {
			vitality := AnalyzeVitality(ticks, series)
			sympathy := AnalyzeSympathy(ticks, series, vitality.Metrics, 20)

			So(sympathy.TotalPairs, ShouldBeGreaterThan, 0)
			So(sympathy.PositivePairs, ShouldBeGreaterThan, 0)
			So(sympathy.InversePairs, ShouldBeGreaterThan, 0)
			So(math.IsNaN(sympathy.NullDistribution.MeanConcordance), ShouldBeFalse)
		})
	})
}

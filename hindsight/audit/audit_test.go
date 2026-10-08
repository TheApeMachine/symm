package audit

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAuditStages(t *testing.T) {
	Convey("Given simulated multi-metric observations", t, func() {
		ticks := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
		rawSeries := make(map[string]map[int64]float64)
		canonicalSeries := make(map[string]map[int64]float64)

		rawSeries["sensor_sin@PEER"] = make(map[int64]float64)
		rawSeries["sensor_clone@PEER"] = make(map[int64]float64)
		rawSeries["sensor_inv@PEER"] = make(map[int64]float64)
		rawSeries["sensor_dead@PEER"] = make(map[int64]float64)

		canonicalSeries["sensor_sin"] = make(map[int64]float64)
		canonicalSeries["sensor_clone"] = make(map[int64]float64)
		canonicalSeries["sensor_inv"] = make(map[int64]float64)
		canonicalSeries["sensor_dead"] = make(map[int64]float64)

		for _, tick := range ticks {
			val := math.Sin(float64(tick) * 0.5)
			rawSeries["sensor_sin@PEER"][tick] = val
			rawSeries["sensor_clone@PEER"][tick] = val + 0.001
			rawSeries["sensor_inv@PEER"][tick] = -val
			rawSeries["sensor_dead@PEER"][tick] = 0.0

			canonicalSeries["sensor_sin"][tick] = val
			canonicalSeries["sensor_clone"][tick] = val + 0.001
			canonicalSeries["sensor_inv"][tick] = -val
			canonicalSeries["sensor_dead"][tick] = 0.0
		}

		Convey("When analyzing contract integrity (Stage 0)", func() {
			meas := data.NewMeasurement(1, "BTC/USD", "test", 1, 1)
			meas = meas.Write(
				data.NewMetric("good_corr", 0.5, data.UnitCorrelation, data.TimescaleTick),
				data.NewMetric("bad_corr", 1.45, data.UnitCorrelation, data.TimescaleTick),
				// The label contains "correlation", but the declared coordinate is a
				// z-score and is therefore not bounded to [-1,1].
				data.NewMetric("correlation_zscore@PEER", 6.2, data.UnitZScore, data.TimescaleTick),
			)

			contract := AnalyzeContract([]*data.Measurement{meas})
			So(contract.TotalMetricsChecked, ShouldEqual, 3)
			So(contract.BreachingMetricsCount, ShouldEqual, 1)
			So(contract.Breaches[0].Metric, ShouldEqual, "bad_corr")
			So(contract.Breaches[0].MaxVal, ShouldAlmostEqual, 1.45, 1e-6)
			So(contract.Passed, ShouldBeFalse)
		})

		Convey("When analyzing metric vitality (Stage 1)", func() {
			vitality := AnalyzeVitality(ticks, rawSeries, canonicalSeries)

			So(vitality.RawProducerMetrics, ShouldEqual, 4)
			So(vitality.RawHealthyMetrics, ShouldEqual, 3)
			So(vitality.CanonicalGridCells, ShouldEqual, 4)
			So(vitality.CanonicalHealthyCells, ShouldEqual, 3)
			So(vitality.CanonicalDeadCells, ShouldEqual, 1)
			So(len(vitality.RedundantPairs), ShouldBeGreaterThanOrEqualTo, 1)
			So(math.Abs(vitality.RedundantPairs[0].Correlation), ShouldBeGreaterThan, 0.99)
		})

		Convey("When analyzing pair sympathy against shuffled null (Stage 2)", func() {
			vitality := AnalyzeVitality(ticks, rawSeries, canonicalSeries)
			sympathy := AnalyzeSympathy(ticks, canonicalSeries, vitality.CanonicalCells, 20)

			So(sympathy.TotalPairs, ShouldBeGreaterThan, 0)
			So(sympathy.PositivePairs, ShouldBeGreaterThan, 0)
			So(sympathy.InversePairs, ShouldBeGreaterThan, 0)
			So(math.IsNaN(sympathy.NullDistribution.MeanConcordance), ShouldBeFalse)
		})

		Convey("When analyzing cognitive trie with insufficient evidence (Stage 6)", func() {
			cognitive := AnalyzeCognitiveTrie(t.Context(), nil, 1, "BTC/USD", nil, nil, nil, 10, nil)

			So(cognitive.Status, ShouldEqual, "INSUFFICIENT_DATA")
			So(cognitive.Passed, ShouldBeFalse)
			So(cognitive.SummaryText, ShouldContainSubstring, "Cognitive Trie: Catalog or grid unavailable.")
		})

		Convey("When testing trie node metric traversal and token deduplication", func() {
			deduped := contextOfTokens([]string{"R1", "R1", "R5", "R12", "R12", "R12"})
			So(deduped, ShouldEqual, "R1/R5/R12")

			emptyDeduped := contextOfTokens(nil)
			So(emptyDeduped, ShouldEqual, "")
		})
	})
}

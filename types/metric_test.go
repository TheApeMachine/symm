package types

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementHypothesisSeparation(t *testing.T) {
	Convey("Given two equally defined sentiment hypotheses", t, func() {
		surge := 0.5
		slump := 0.5
		separation := MeasurementHypothesisSeparation(SourceSentiment, map[string]MetricSample{
			MetricKey(MetricSurgeScore, SideNone): {Normalized: &surge},
			MetricKey(MetricSlumpScore, SideNone): {Normalized: &slump},
		})

		Convey("It reports no separation between the competing hypotheses", func() {
			So(separation, ShouldEqual, 0.0)
		})
	})

	Convey("Given complementary metrics supporting one toxicity group", t, func() {
		firstExecution := 0.6
		secondExecution := 0.8
		retreat := math.Sqrt(0.5)
		separation := MeasurementHypothesisSeparation(SourceToxicity, map[string]MetricSample{
			MetricKey(MetricFillVolume, SideBuy):  {Normalized: &firstExecution},
			MetricKey(MetricFillVolume, SideSell): {Normalized: &secondExecution},
			MetricKey(MetricRetreatingQuantity, SideBuy): {
				Normalized: &retreat,
			},
		})

		Convey("It combines support without rewarding the larger group", func() {
			So(separation, ShouldAlmostEqual, 0, 1e-12)
		})
	})

	Convey("Given several non-winning correlation hypotheses", t, func() {
		herd := 0.9
		alpha := 0.3
		noise := 0.4
		stress := 0.0
		separation := MeasurementHypothesisSeparation(SourceCorrelation, map[string]MetricSample{
			MetricKey(MetricHerdScore, SideNone):   {Normalized: &herd},
			MetricKey(MetricAlphaScore, SideNone):  {Normalized: &alpha},
			MetricKey(MetricNoiseScore, SideNone):  {Normalized: &noise},
			MetricKey(MetricStressScore, SideNone): {Normalized: &stress},
		})

		Convey("It uses the total competing energy as the alternative floor", func() {
			So(separation, ShouldAlmostEqual, (0.9-0.5)/0.9, 1e-12)
		})
	})

	Convey("Given one active hypothesis and explicitly measured zero alternatives", t, func() {
		herd := 0.9
		inactive := 0.0
		separation := MeasurementHypothesisSeparation(
			SourceCorrelation,
			map[string]MetricSample{
				MetricKey(MetricHerdScore, SideNone):   {Normalized: &herd},
				MetricKey(MetricAlphaScore, SideNone):  {Normalized: &inactive},
				MetricKey(MetricNoiseScore, SideNone):  {Normalized: &inactive},
				MetricKey(MetricStressScore, SideNone): {Normalized: &inactive},
			},
		)

		Convey("It reports complete category separation without claiming perfect precision", func() {
			So(separation, ShouldEqual, 1.0)
		})
	})

	Convey("Given strong sentiment context beside directional hypotheses", t, func() {
		surge := 0.8
		slump := 0.2
		divergence := 1.0
		leaderEvidence := 1.0
		separation := MeasurementHypothesisSeparation(SourceSentiment, map[string]MetricSample{
			MetricKey(MetricSurgeScore, SideNone):     {Normalized: &surge},
			MetricKey(MetricSlumpScore, SideNone):     {Normalized: &slump},
			MetricKey(MetricDivergentScore, SideNone): {Normalized: &divergence},
			MetricKey(MetricLeaderEvidence, SideNone): {Normalized: &leaderEvidence},
		})

		Convey("It does not turn complementary context into competing noise", func() {
			So(separation, ShouldAlmostEqual, 0.75, 1e-12)
		})
	})

	Convey("Given fewer than two measured competing groups", t, func() {
		herd := 0.9
		separation := MeasurementHypothesisSeparation(SourceCorrelation, map[string]MetricSample{
			MetricKey(MetricHerdScore, SideNone): {Normalized: &herd},
		})

		Convey("It leaves hypothesis separation undefined", func() {
			So(separation, ShouldEqual, 0.0)
		})
	})

	Convey("Given an emitted metric without an explicit group", t, func() {
		Convey("It fails loudly", func() {
			So(func() {
				MeasurementHypothesisSeparation(SourceCVD, map[string]MetricSample{
					"unmapped": {},
				})
			}, ShouldPanic)
		})
	})
}

func BenchmarkMeasurementHypothesisSeparation(b *testing.B) {
	herd := 0.9
	alpha := 0.3
	noise := 0.4
	stress := 0.0
	metrics := map[string]MetricSample{
		MetricKey(MetricHerdScore, SideNone):   {Normalized: &herd},
		MetricKey(MetricAlphaScore, SideNone):  {Normalized: &alpha},
		MetricKey(MetricNoiseScore, SideNone):  {Normalized: &noise},
		MetricKey(MetricStressScore, SideNone): {Normalized: &stress},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = MeasurementHypothesisSeparation(SourceCorrelation, metrics)
	}
}

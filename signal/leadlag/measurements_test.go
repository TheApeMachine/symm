package leadlag

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSelectCorrelations(t *testing.T) {
	Convey("Given supported lag candidates", t, func() {
		signal := &Signal{section: NewSection()}
		anchorLeading := LagFeatures{
			LagOK: true, LagBars: 1, LagCorr: 0.95, SampleCount: 64,
		}
		followerLeading := LagFeatures{
			LagOK: true, LagBars: -1, LagCorr: 0.95, SampleCount: 64,
		}
		antiCorrelated := LagFeatures{
			LagOK: true, LagBars: 1, LagCorr: -0.95, SampleCount: 64,
		}

		Convey("It should retain only positive anchor-leading evidence", func() {
			selected := signal.selectCorrelations(anchorLeading)
			So(selected.signedLagCorrelation, ShouldEqual, 0.95)
			So(selected.lagDirection, ShouldEqual, 1)
			So(signal.selectCorrelations(followerLeading).signedLagCorrelation, ShouldEqual, 0)
			So(signal.selectCorrelations(antiCorrelated).signedLagCorrelation, ShouldEqual, 0)
		})
	})
}

func TestLagSearchThreshold(t *testing.T) {
	Convey("Given the same effective support", t, func() {
		Convey("It should equal the multiple-search bound exactly", func() {
			So(lagSearchThreshold(64, 16), ShouldEqual,
				math.Sqrt(2*math.Log(16)/64))
			So(lagSearchThreshold(64, 2), ShouldEqual,
				math.Sqrt(2*math.Log(2)/64))
			So(lagSearchThreshold(0, 16), ShouldEqual, 0.0)
			So(lagSearchThreshold(64, 1), ShouldEqual, 0.0)
		})
	})
}

func TestSampleSupportFraction(t *testing.T) {
	Convey("Given price counts around the first complete return window", t, func() {
		Convey("It should count returns rather than treating the origin as evidence", func() {
			So(sampleSupportFraction(-1), ShouldEqual, 0.0)
			So(sampleSupportFraction(0), ShouldEqual, 0.0)
			So(sampleSupportFraction(1), ShouldEqual, 0.0)
			So(sampleSupportFraction(2), ShouldEqual, 0.5)
			So(sampleSupportFraction(3), ShouldEqual, 1.0)
			So(sampleSupportFraction(4), ShouldEqual, 1.0)
		})
	})
}

func TestBuildScoreMeasurement(t *testing.T) {
	Convey("Given supported, bounded lead-lag evidence", t, func() {
		measurement := buildScoreMeasurement(
			"ALT/USD",
			"BTC/USD",
			time.Unix(1_700_020_000, 0).UTC(),
			correlationSelection{
				correlation: 0.8, signedCorrelation: -0.8,
				signedContempCorrelation: -0.8, lagFraction: 0.25,
				lagDirection: 1,
			},
			1,
			evidenceWeights{syncScore: 0.2, strength: 0.2},
		)

		Convey("It should expose the equation's dimensionless values", func() {
			So(*measurement.Sample(types.MetricSignedCorrelation, types.SideNone).Normalized,
				ShouldEqual, -0.8)
			So(*measurement.Sample(types.MetricLagFraction, types.SideNone).Normalized,
				ShouldEqual, 0.25)
			So(measurement.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, 1.0)
			So(measurement.Metrics, ShouldHaveLength, 13)
			So(measurement.Symbol, ShouldEqual, "ALT/USD")
			So(measurement.Peer, ShouldEqual, "BTC/USD")

			for _, sample := range measurement.Metrics {
				So(sample.Unit, ShouldEqual, types.UnitDimensionless)
			}
		})
	})

	Convey("Given tied lead-lag hypotheses", t, func() {
		measurement := buildScoreMeasurement(
			"ALT/USD",
			"BTC/USD",
			time.Unix(1_700_020_001, 0).UTC(),
			correlationSelection{},
			1,
			evidenceWeights{inefficient: 0.5, syncScore: 0.5, strength: 0.5},
		)

		Convey("It should report zero SNR", func() {
			So(measurement.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, 0.0)
		})
	})
}

func TestWeightEvidence(t *testing.T) {
	Convey("Given exact lag, contemporaneous, support, and stall fractions", t, func() {
		features := LagFeatures{
			MoveReady:   true,
			StallMargin: 0.25,
		}
		selected := correlationSelection{
			signedCorrelation:        0.8,
			signedContempCorrelation: 0.4,
			signedLagCorrelation:     0.8,
			lagFraction:              0.5,
		}
		weights := weightEvidence(features, selected, 0.75)

		Convey("It should apply each documented factor without category leakage", func() {
			sampleSupport := 0.75
			stallMargin := features.StallMargin
			noLag := 1 - selected.lagFraction
			uncorrelated := 1 - selected.signedCorrelation
			lagEvidence := selected.signedLagCorrelation * selected.lagFraction
			syncEvidence := selected.signedContempCorrelation * noLag
			So(weights.inefficient, ShouldEqual,
				sampleSupport*lagEvidence*(1-stallMargin))
			So(weights.syncScore, ShouldEqual,
				sampleSupport*syncEvidence*(1-stallMargin))
			So(weights.decoupled, ShouldEqual,
				sampleSupport*uncorrelated*(1-stallMargin))
			So(weights.stall, ShouldEqual,
				sampleSupport*stallMargin*uncorrelated*noLag)
			So(weights.strength, ShouldEqual, weights.inefficient)
		})

		Convey("It should remove stall evidence after the anchor actually moves", func() {
			features.MoveMoved = true
			So(weightEvidence(features, selected, 0.75).stall, ShouldEqual, 0.0)
		})
	})
}

func TestNormalizedLeadLag(t *testing.T) {
	Convey("Given lead-lag's signed, unsigned, and nominal domains", t, func() {
		Convey("It should retain every exact boundary", func() {
			So(*normalizedLeadLag(types.MetricCorrelation, 0), ShouldEqual, 0.0)
			So(*normalizedLeadLag(types.MetricCorrelation, 1), ShouldEqual, 1.0)
			So(*normalizedLeadLag(types.MetricSignedCorrelation, -1), ShouldEqual, -1.0)
			So(*normalizedLeadLag(types.MetricSignedCorrelation, 1), ShouldEqual, 1.0)
			So(*normalizedLeadLag(types.MetricSignedLagDirection, -1), ShouldEqual, -1.0)
			So(*normalizedLeadLag(types.MetricSignedLagDirection, 0), ShouldEqual, 0.0)
			So(*normalizedLeadLag(types.MetricSignedLagDirection, 1), ShouldEqual, 1.0)
		})

		Convey("It should reject the nearest exterior values and fractional direction", func() {
			So(normalizedLeadLag(
				types.MetricCorrelation,
				math.Nextafter(0, -1),
			), ShouldBeNil)
			So(normalizedLeadLag(
				types.MetricCorrelation,
				math.Nextafter(1, 2),
			), ShouldBeNil)
			So(normalizedLeadLag(
				types.MetricSignedCorrelation,
				math.Nextafter(-1, -2),
			), ShouldBeNil)
			So(normalizedLeadLag(
				types.MetricSignedCorrelation,
				math.Nextafter(1, 2),
			), ShouldBeNil)
			So(normalizedLeadLag(
				types.MetricSignedLagDirection,
				0.5,
			), ShouldBeNil)
		})
	})
}

func BenchmarkBuildScoreMeasurement(b *testing.B) {
	selected := correlationSelection{
		correlation: 0.8, signedCorrelation: 0.8,
		signedContempCorrelation: 0.8, lagFraction: 0.25, lagDirection: 1,
	}
	weights := evidenceWeights{inefficient: 0.2, syncScore: 0.3, strength: 0.3}
	at := time.Unix(1_700_020_200, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		_ = buildScoreMeasurement("ALT/USD", "BTC/USD", at, selected, 1, weights)
	}
}

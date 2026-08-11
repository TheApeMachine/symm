package leadlag

import (
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
			So(selected.signedLagCorrelation, ShouldAlmostEqual, 0.95)
			So(selected.lagDirection, ShouldEqual, 1)
			So(signal.selectCorrelations(followerLeading).signedLagCorrelation, ShouldEqual, 0)
			So(signal.selectCorrelations(antiCorrelated).signedLagCorrelation, ShouldEqual, 0)
		})
	})
}

func TestLagSearchThreshold(t *testing.T) {
	Convey("Given the same effective support", t, func() {
		Convey("It should penalize a wider lag search", func() {
			So(lagSearchThreshold(64, 16), ShouldBeGreaterThan,
				lagSearchThreshold(64, 2))
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
				ShouldAlmostEqual, -0.8, 1e-12)
			So(*measurement.Sample(types.MetricLagFraction, types.SideNone).Normalized,
				ShouldAlmostEqual, 0.25, 1e-12)
			So(measurement.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, 1.0)
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

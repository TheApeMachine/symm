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
			8,
			correlationSelection{
				correlation: 0.8, signedCorrelation: -0.8,
				signedContempCorrelation: -0.8, lagFraction: 0.25,
				lagDirection: 1,
			},
			1,
			evidenceWeights{syncScore: 0.2, strength: 0.2},
		)

		Convey("It should expose the equation's dimensionless values", func() {
			So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
			So(*measurement.Sample(types.MetricSignedCorrelation, types.SideNone).Normalized,
				ShouldAlmostEqual, -0.8, 1e-12)
			So(*measurement.Sample(types.MetricLagFraction, types.SideNone).Normalized,
				ShouldAlmostEqual, 0.25, 1e-12)
		})
	})

	Convey("Given only one observation", t, func() {
		measurement := buildScoreMeasurement(
			"ALT/USD", "BTC/USD", time.Unix(1_700_020_100, 0).UTC(), 1,
			correlationSelection{correlation: 0.8}, 1, evidenceWeights{},
		)

		Convey("It should keep normalized evidence absent while provisional", func() {
			So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(measurement.Sample(types.MetricCorrelation, types.SideNone).Normalized,
				ShouldBeNil)
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

	for range b.N {
		_ = buildScoreMeasurement("ALT/USD", "BTC/USD", at, 8, selected, 1, weights)
	}
}

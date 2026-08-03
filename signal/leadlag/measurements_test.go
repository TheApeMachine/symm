package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

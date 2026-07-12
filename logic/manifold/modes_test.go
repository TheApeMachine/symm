package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func TestModeExtractorSpectrumAnchorPositions(t *testing.T) {
	Convey("Given a stationary cohort in centered price-size and unit-age coordinates", t, func() {
		config := &pmanifold.Config{
			GridX: 10, GridY: 8, GridZ: 16,
			DomainX: 10, DomainY: 8, DomainZ: 1,
			DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 4,
		}
		pmanifold.ApplyDerivedGasParams(config)
		extractor := NewModeExtractor(config)
		cohorts := []Cohort{{
			Side:     OrderSideBid,
			Mass:     2,
			Centroid: Coordinate{Price: -1, Size: 1, Age: 0.25},
		}}

		modes := extractor.SpectrumAnchor(cohorts, 0.2)

		Convey("It should emit physical positions inside each configured domain", func() {
			So(modes, ShouldHaveLength, 1)
			So(modes[0].PosX, ShouldEqual, 4.0)
			So(modes[0].PosY, ShouldEqual, 5.0)
			So(modes[0].PosZ, ShouldEqual, 0.25)
		})
	})
}

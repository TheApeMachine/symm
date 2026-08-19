package opportunity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCatalog(t *testing.T) {
	Convey("Given the opportunity catalog", t, func() {
		Convey("It should define each identifiable opportunity exactly once", func() {
			seen := map[types.OpportunityType]bool{}

			for _, archetype := range Catalog {
				So(archetype.Type, ShouldNotEqual, types.OpportunityNone)
				So(seen[archetype.Type], ShouldBeFalse)
				So(archetype.RolloutDynamics, ShouldNotBeEmpty)
				seen[archetype.Type] = true
			}

			So(seen[types.OpportunitySuddenPump], ShouldBeTrue)
			So(seen[types.OpportunityCoiledCompression], ShouldBeTrue)
			So(seen[types.OpportunityDailyRiser], ShouldBeTrue)
			So(seen[types.OpportunityInefficientLag], ShouldBeTrue)
			So(seen[types.OpportunityAbsorptionReversal], ShouldBeTrue)
		})

		Convey("Every supporting leg should be a supporting condition", func() {
			for _, archetype := range Catalog {
				for _, leg := range archetype.Supports {
					So(leg.Supports, ShouldBeTrue)
					So(leg.Contradicts, ShouldBeFalse)
					So(leg.Source, ShouldNotBeEmpty)
					So(leg.Metric, ShouldNotBeEmpty)
				}
			}
		})

		Convey("Every opposing leg should be a contradicting condition", func() {
			for _, archetype := range Catalog {
				for _, leg := range archetype.Opposes {
					So(leg.Contradicts, ShouldBeTrue)
					So(leg.Supports, ShouldBeFalse)
					So(leg.Source, ShouldNotBeEmpty)
					So(leg.Metric, ShouldNotBeEmpty)
				}
			}
		})
	})
}

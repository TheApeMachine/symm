package trader

import (
	"strings"
	"testing"

	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTopologySequence(testingTB *testing.T) {
	Convey("Given two observations with the same readable category", testingTB, func() {
		topology := newTopology()
		low := &cortexObservation{
			symbol: "BTC/USD",
			measurements: map[types.SourceType]types.Category{
				types.SourceFluid: {
					Type:       types.CategoryLaminar,
					Confidence: 0.25,
					Strength:   0.20,
				},
			},
		}
		high := &cortexObservation{
			symbol: "BTC/USD",
			measurements: map[types.SourceType]types.Category{
				types.SourceFluid: {
					Type:       types.CategoryLaminar,
					Confidence: 0.85,
					Strength:   0.80,
				},
			},
		}

		Convey("When the topology maps them into DMT sequences", func() {
			lowSequence, lowErr := topology.Sequence(low)
			highSequence, highErr := topology.Sequence(high)

			Convey("Then the UI label stays readable while the tree token carries the continuous state", func() {
				So(lowErr, ShouldBeNil)
				So(highErr, ShouldBeNil)
				So(lowSequence.Display, ShouldResemble, highSequence.Display)
				So(lowSequence.Display[0], ShouldEqual, "fluid-laminar")
				So(lowSequence.Tree[0], ShouldNotEqual, highSequence.Tree[0])
				So(strings.HasPrefix(lowSequence.Tree[0], "m"), ShouldBeTrue)
				So(topology.Label(lowSequence.Tree[0]), ShouldEqual, "fluid-laminar")
			})
		})
	})
}

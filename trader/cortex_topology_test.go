package trader

import (
	"testing"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTopologySequence(testingTB *testing.T) {
	Convey("Given two observations with the same readable category", testingTB, func() {
		topology := newTopology()
		low := &cortexObservation{
			symbol: "BTC/USD",
			measurements: map[types.SourceType]cortexReading{
				types.SourceFluid: {
					category: types.Category{
						Type:       types.CategoryLaminar,
						Confidence: 0.25,
						Strength:   0.20,
					},
				},
			},
		}
		high := &cortexObservation{
			symbol: "BTC/USD",
			measurements: map[types.SourceType]cortexReading{
				types.SourceFluid: {
					category: types.Category{
						Type:       types.CategoryLaminar,
						Confidence: 0.85,
						Strength:   0.80,
					},
				},
			},
		}

		Convey("When the topology maps them into DMT sequences", func() {
			lowSequence, lowErr := topology.Sequence(low)
			highSequence, highErr := topology.Sequence(high)

			Convey("Then the same category maps to the same tree token regardless of magnitude", func() {
				So(lowErr, ShouldBeNil)
				So(highErr, ShouldBeNil)
				So(lowSequence.Display, ShouldResemble, highSequence.Display)
				So(lowSequence.Display[0], ShouldEqual, "fluid-laminar")
				// The trie keeps category-transition memory, not magnitude, so
				// low and high intensity of the same category collapse to one
				// node. Magnitude lives in the predictive-coding step.
				So(lowSequence.Tree[0], ShouldEqual, highSequence.Tree[0])
				So(lowSequence.Tree[0], ShouldEqual, "fluid-laminar")
				So(topology.Label(lowSequence.Tree[0]), ShouldEqual, "fluid-laminar")
			})
		})
	})

	Convey("Given a manifold frame with physical field evidence", testingTB, func() {
		topology := newTopology()
		observation := &cortexObservation{
			symbol:       "BTC/USD",
			measurements: map[types.SourceType]cortexReading{},
			manifold: &logic.ManifoldFrame{
				Category: types.CategoryPhysicalField,
				Strength: 0.8,
				Momentum: 0.4,
				Pressure: 0.3,
				Shock:    0.2,
			},
		}

		Convey("When the topology maps it into a DMT sequence", func() {
			sequence, err := topology.Sequence(observation)

			Convey("Then physical field uses the canonical category index", func() {
				So(err, ShouldBeNil)
				So(sequence.Display, ShouldResemble, []string{"manifold-physical-field"})
				So(sequence.Tree[0], ShouldEqual, "manifold-physical-field")
				So(topology.Label(sequence.Tree[0]), ShouldEqual, "manifold-physical-field")
			})
		})
	})

	Convey("Given a manifold frame with no category (sparse fallback)", testingTB, func() {
		// A price_zero source emits a minimal manifold frame with no category.
		// The topology must skip it, not error — an error here fails cortex
		// Measure for the whole tick and short-circuits the trade loop.
		topology := newTopology()
		observation := &cortexObservation{
			symbol:       "BTC/USD",
			measurements: map[types.SourceType]cortexReading{},
			manifold: &logic.ManifoldFrame{
				Category: types.CategoryTypeNone,
				Momentum: 0.4,
				Pressure: 0.3,
			},
		}

		Convey("When the topology maps it into a DMT sequence", func() {
			sequence, err := topology.Sequence(observation)

			Convey("Then it is skipped without error", func() {
				So(err, ShouldBeNil)
				So(sequence.Display, ShouldBeEmpty)
			})
		})
	})
}

package advisor

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

func TestRelativeStateStep(t *testing.T) {
	Convey("Given a RelativeStateAdvisor", t, func() {
		advisor := NewRelativeStateAdvisor(t.Context(), nil)

		Convey("an empty batch produces no perspective", func() {
			So(advisor.Step(nil), ShouldBeNil)
			So(advisor.Step([]types.Category{}), ShouldBeNil)
		})

		Convey("the first symbol is its own whole population", func() {
			perspective := advisor.Step(singleCategory("TEST/USD", time.Unix(0, 0), types.VerticalIgnition, 1.0))

			So(perspective, ShouldNotBeNil)
			So(perspective.Kind, ShouldEqual, types.KindRelativeState)
			So(perspective.Relative.PeerCount, ShouldEqual, 1)
			So(perspective.Relative.SameRegime, ShouldEqual, 1)
			So(perspective.Relative.Breadth, ShouldEqual, 1.0)
			So(perspective.Relative.MajorityRegime, ShouldEqual, uint8(types.CategoryIndex(types.VerticalIgnition)))
		})

		Convey("breadth reflects the fraction of the population sharing the regime", func() {
			advisor.Step(singleCategory("A/USD", time.Unix(0, 0), types.VerticalIgnition, 1.0))
			advisor.Step(singleCategory("B/USD", time.Unix(0, 1), types.VerticalIgnition, 1.0))
			perspective := advisor.Step(singleCategory("C/USD", time.Unix(0, 2), types.CoiledCompression, 1.0))

			So(perspective.Relative.PeerCount, ShouldEqual, 3)
			So(perspective.Relative.SameRegime, ShouldEqual, 1)
			So(perspective.Relative.Breadth, ShouldEqual, 1.0/3.0)

			coiled := uint8(types.CategoryIndex(types.CoiledCompression))
			vertical := uint8(types.CategoryIndex(types.VerticalIgnition))
			So(coiled, ShouldNotEqual, vertical)
			So(perspective.Relative.MajorityRegime, ShouldEqual, vertical)
			So(perspective.Relative.MajorityBreadth, ShouldEqual, 2.0/3.0)
		})

		Convey("a regime change updates the population counts", func() {
			advisor.Step(singleCategory("TEST/USD", time.Unix(0, 0), types.VerticalIgnition, 1.0))
			advisor.Step(singleCategory("OTHER/USD", time.Unix(0, 1), types.CoiledCompression, 1.0))

			perspective := advisor.Step(singleCategory("TEST/USD", time.Unix(0, 2), types.CoiledCompression, 1.0))

			So(perspective.Relative.PeerCount, ShouldEqual, 2)
			So(perspective.Relative.SameRegime, ShouldEqual, 2)
			So(perspective.Relative.Breadth, ShouldEqual, 1.0)
		})

		Convey("two advisors fed identical batches emit identical perspectives", func() {
			left := NewRelativeStateAdvisor(t.Context(), nil)
			right := NewRelativeStateAdvisor(t.Context(), nil)

			feed := func(advisor *RelativeStateAdvisor) *types.Perspective {
				advisor.Step(singleCategory("A/USD", time.Unix(0, 0), types.VerticalIgnition, 1.0))
				advisor.Step(singleCategory("B/USD", time.Unix(0, 1), types.CoiledCompression, 1.0))
				return advisor.Step(singleCategory("C/USD", time.Unix(0, 2), types.VerticalIgnition, 1.0))
			}

			leftPerspective := feed(left)
			rightPerspective := feed(right)

			So(leftPerspective.Relative, ShouldResemble, rightPerspective.Relative)
			So(leftPerspective.Relative.MajorityRegime, ShouldEqual, rightPerspective.Relative.MajorityRegime)
		})
	})
}

func TestObserve(t *testing.T) {
	Convey("Given an empty population", t, func() {
		advisor := NewRelativeStateAdvisor(context.Background(), nil)

		Convey("observing a symbol registers its regime", func() {
			advisor.observe("TEST/USD", 3)
			So(advisor.regimes["TEST/USD"], ShouldEqual, 3)
			So(advisor.counts[3], ShouldEqual, 1)
		})

		Convey("re-observing the same regime does not double count", func() {
			advisor.observe("TEST/USD", 3)
			advisor.observe("TEST/USD", 3)
			So(advisor.counts[3], ShouldEqual, 1)
		})

		Convey("a regime change decrements the old and increments the new", func() {
			advisor.observe("TEST/USD", 3)
			advisor.observe("TEST/USD", 5)
			So(advisor.counts[3], ShouldEqual, 0)
			So(advisor.counts[5], ShouldEqual, 1)
			So(advisor.regimes["TEST/USD"], ShouldEqual, 5)
		})
	})
}

func TestMajority(t *testing.T) {
	Convey("Given a population count table", t, func() {
		advisor := NewRelativeStateAdvisor(context.Background(), nil)

		Convey("the most frequent regime is returned", func() {
			advisor.counts[1] = 5
			advisor.counts[2] = 2
			regime, count := advisor.majority()
			So(regime, ShouldEqual, 1)
			So(count, ShouldEqual, 5)
		})

		Convey("ties break by the lowest interned index", func() {
			advisor.counts[2] = 3
			advisor.counts[1] = 3
			regime, _ := advisor.majority()
			So(regime, ShouldEqual, 1)
		})
	})
}

func TestShare(t *testing.T) {
	Convey("Given a count and total", t, func() {
		So(share(0, 0), ShouldEqual, 0)
		So(share(1, 0), ShouldEqual, 0)
		So(share(1, 4), ShouldEqual, 0.25)
		So(share(4, 4), ShouldEqual, 1)
	})
}

func TestPopulationMaturity(t *testing.T) {
	Convey("Given a population size", t, func() {
		So(populationMaturity(0), ShouldEqual, 0)
		So(populationMaturity(1), ShouldEqual, 0)
		So(populationMaturity(2), ShouldEqual, 0.5)
		So(populationMaturity(4), ShouldEqual, 0.75)
	})
}

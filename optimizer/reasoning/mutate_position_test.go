package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestTunePositionSize(t *testing.T) {
	Convey("Given a forest with a market entry", t, func() {
		forest := []reasoning.Thought{{
			When: allOf(notHolding(), signalAtLeast(types.CategoryVerticalIgnition, 1)),
			Do:   reasoning.Act{Type: reasoning.ActionMarket, Fraction: 1},
		}}
		vocab := Vocabulary{Fractions: []float64{0.5, 1, 2}}

		neighbors := tunePositionSize(forest, vocab)

		Convey("It should propose alternate capital multipliers", func() {
			So(len(neighbors), ShouldEqual, 2)
			So(neighbors[0][0].Do.Fraction, ShouldNotEqual, 1)
		})
	})
}

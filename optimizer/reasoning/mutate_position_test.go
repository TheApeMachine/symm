package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTunePositionSize(t *testing.T) {
	Convey("Given a forest with a market entry", t, func() {
		forest := []perspectives.Thought{{
			When: allOf(notHolding(), signalAtLeast(perspectives.CategoryVerticalIgnition, 1)),
			Do:   perspectives.Act{Type: perspectives.ActionMarket, Fraction: 1},
		}}
		vocab := Vocabulary{Fractions: []float64{0.5, 1, 2}}

		neighbors := tunePositionSize(forest, vocab)

		Convey("It should propose alternate capital multipliers", func() {
			So(len(neighbors), ShouldEqual, 2)
			So(neighbors[0][0].Do.Fraction, ShouldNotEqual, 1)
		})
	})
}

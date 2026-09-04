package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/* counting is a stateful node that totals what it is stepped with, once per observation. */
type counting struct {
	Guard
	total Scalar
}

func (node *counting) Step(x Scalar) Scalar {
	if !node.Fresh() {
		return node.total
	}

	node.total += x

	return node.total
}

func TestGuardHoldsTheDiamond(t *testing.T) {
	Convey("Given one accumulation two derived quantities both depend on", t, func() {
		// The shape that forced the scratchpad: gross = buy + sell and
		// net = buy - sell both reach the same accumulator, and their ratio
		// reaches it again through both.
		shared := &counting{}
		tick := &Tick{}
		Bind(shared, tick)

		Convey("reaching it three times in one observation counts once", func() {
			tick.Advance()

			So(shared.Step(5), ShouldEqual, Scalar(5))
			So(shared.Step(5), ShouldEqual, Scalar(5))
			So(shared.Step(5), ShouldEqual, Scalar(5))
		})

		Convey("the next observation advances it again", func() {
			tick.Advance()
			shared.Step(5)
			shared.Step(5)

			tick.Advance()
			So(shared.Step(3), ShouldEqual, Scalar(8))
			So(shared.Step(3), ShouldEqual, Scalar(8))
		})
	})

	Convey("Given a stateful node used on its own", t, func() {
		orphan := &counting{}

		Convey("it advances every step: no graph can reach it twice", func() {
			So(orphan.Step(5), ShouldEqual, Scalar(5))
			So(orphan.Step(5), ShouldEqual, Scalar(10))
		})
	})
}

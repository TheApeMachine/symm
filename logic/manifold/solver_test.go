package manifold

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAdmit(t *testing.T) {
	Convey("Given a manifold carrier lattice at capacity", t, func() {
		solver := &Solver{}

		for index := range phaseLatticeWidth {
			So(solver.admit(fmt.Sprintf("SYMBOL-%d", index)), ShouldBeTrue)
		}

		Convey("It should retain the resident universe and reject another carrier", func() {
			So(solver.admit("OVERFLOW"), ShouldBeFalse)
			So(solver.universe, ShouldHaveLength, int(phaseLatticeWidth))
			So(solver.admit("SYMBOL-0"), ShouldBeTrue)
			So(solver.universe, ShouldHaveLength, int(phaseLatticeWidth))
		})
	})
}

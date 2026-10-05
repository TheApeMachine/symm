package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestExpNext(t *testing.T) {
	Convey("Exp speaks only exponent and exponential", t, func() {
		mapping := data.NewMap(
			"exponent", "input",
			"exponential", "output",
		)

		value, ok := driveAdapter(NewExp(), mapping, map[string]float64{"input": 1}, "output")
		So(ok, ShouldBeTrue)
		So(value, ShouldAlmostEqual, math.E)
	})
}

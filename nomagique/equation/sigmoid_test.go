package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestSigmoid(t *testing.T) {
	Convey("Sigmoid maps x through the logistic function σ(x)=1/(1+e^-x)", t, func() {
		cases := []struct{ x, want float64 }{
			{0, 0.5},
			{1, 1 / (1 + math.Exp(-1))},
			{-1, 1 / (1 + math.Exp(1))},
		}

		for _, c := range cases {
			input := types.Frame{}.Set(calculus.PortX, c.x)
			output := Sigmoid()(input)

			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolSigmoid), ShouldAlmostEqual, c.want, 1e-12)
		}
	})
}

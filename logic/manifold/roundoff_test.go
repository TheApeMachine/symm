package manifold

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoundedQuantityAdd(t *testing.T) {
	Convey("Given additions at a binary64 binade boundary", t, func() {
		quantity := roundedQuantity{}
		quantity = quantity.Add(roundedQuantity{value: math.Exp2(53)})
		quantity = quantity.Add(roundedQuantity{value: 1})
		quantity = quantity.Add(roundedQuantity{value: 1})

		Convey("It should enclose both units lost to rounding", func() {
			So(quantity.value, ShouldEqual, math.Exp2(53))
			So(quantity.roundoff, ShouldBeGreaterThanOrEqualTo, 2)
			So(roundingRadius(math.Exp2(53)), ShouldEqual, 1)
		})
	})
}

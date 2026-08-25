package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

const accValueSlot = "test/value"
const accTotalSlot = "test/total"
const accCountSlot = "test/count"

func TestAccumulator(t *testing.T) {
	Convey("Given an accumulator primitive over a prefixed series", t, func() {
		accumulator := Accumulator("test")

		Convey("It integrates signed samples with compensated summation", func() {
			number := nomagique.NewNumber[string](accumulator)

			_ = number.Step("sym", withValue(accValueSlot, 0.1))
			_ = number.Step("sym", withValue(accValueSlot, 0.2))
			output := number.Step("sym", withValue(accValueSlot, 0.3))

			So(output.Err, ShouldBeNil)
			So(output.MustGet(types.MustIntern(accTotalSlot)), ShouldEqual, 0.6)
			So(output.MustGet(types.MustIntern(accCountSlot)), ShouldEqual, 3.0)
		})

		Convey("It rejects a missing value", func() {
			output := accumulator(types.Frame{})
			So(output.Err, ShouldNotBeNil)
		})

		Convey("It rejects a non-finite sample", func() {
			output := accumulator(withValue(accValueSlot, math.Inf(1)))
			So(output.Err, ShouldNotBeNil)
		})
	})
}

package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestDeviation(t *testing.T) {
	Convey("Given a signed residual against a negative baseline", t, func() {
		input := types.Frame{}
		input.Put(nomagique.SampleValue, -4.0)
		input.Put(SymbolBaselineValue, -8.0)
		_, output, err := Deviation(types.Frame{}, input)
		So(err, ShouldBeNil)

		Convey("It should scale by the baseline's magnitude instead of reporting zero", func() {
			So(output.MustGet(SymbolDeviation), ShouldEqual, 0.5)
		})
	})

	Convey("Given a zero baseline", t, func() {
		input := types.Frame{}
		input.Put(nomagique.SampleValue, 3.0)
		input.Put(SymbolBaselineValue, 0.0)
		_, output, err := Deviation(types.Frame{}, input)
		So(err, ShouldBeNil)

		Convey("It should report zero because there is no scale", func() {
			So(output.MustGet(SymbolDeviation), ShouldEqual, 0)
		})
	})
}

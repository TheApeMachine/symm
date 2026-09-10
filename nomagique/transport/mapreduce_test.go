package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMapReduce(t *testing.T) {
	Convey("MapReduce is mapper then reducer over one iterator", t, func() {
		out := tests.CollectSeq(MapReduce(
			calculus.NewSquare[float64](),
			arithmetic.NewAdd[float64, float64](0.0),
			Values(1.0, 2.0, 3.0, 4.0),
		))

		So(out[len(out)-1], ShouldEqual, 30)
	})
}

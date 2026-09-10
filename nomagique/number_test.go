package nomagique_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNumber(t *testing.T) {
	Convey("Number threads a run through homogeneous stages", t, func() {
		add := arithmetic.NewAdd[float64, float64](2.0)
		multiply := arithmetic.NewMultiply[float64](3.0)
		out := tests.CollectSeq(multiply.Next(nomagique.Number(transport.Values(5.0), add)))

		So(out[0], ShouldEqual, 21)
	})
}

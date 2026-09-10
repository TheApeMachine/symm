package collection

import (
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestOrderNext(t *testing.T) {
	Convey("Order sorts a clone and leaves the input collection alone", t, func() {
		input := []float64{3, 1, 2}
		op := NewOrder[float64]()
		out := tests.CollectSeq(op.Next(transport.Values(input)))

		So(out[0], ShouldResemble, []float64{1, 2, 3})
		So(slices.Equal(input, []float64{3, 1, 2}), ShouldBeTrue)
		So(op.Error(), ShouldBeNil)
	})
}

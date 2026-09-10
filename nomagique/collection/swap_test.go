package collection

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSwapNext(t *testing.T) {
	Convey("Swap exchanges two configured indices without mutating the input", t, func() {
		op := NewSwap[float64](0, 2)
		original := []float64{2, 3, 4}
		out := tests.CollectSeq(op.Next(transport.Values(original)))

		So(out[0], ShouldResemble, []float64{4, 3, 2})
		So(original[0], ShouldEqual, 2)
		So(op.Error(), ShouldBeNil)
	})
}

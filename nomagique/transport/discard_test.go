package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDiscardNext(t *testing.T) {
	Convey("Discard consumes a run and hands nothing over", t, func() {
		op := NewDiscard[float64]()
		out := tests.CollectSeq(op.Next(Values(1.0, 2.0, 3.0)))

		So(len(out), ShouldEqual, 0)
		So(op.Error(), ShouldBeNil)
	})
}

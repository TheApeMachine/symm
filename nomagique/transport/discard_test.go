package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDiscardNext(t *testing.T) {
	Convey("Discard consumes a run and hands nothing over", t, func() {
		op := transport.NewDiscard()
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(1.0, 2.0, 3.0).Next(nil)))

		So(len(out), ShouldEqual, 0)
		So(op.Error(), ShouldBeNil)
	})
}

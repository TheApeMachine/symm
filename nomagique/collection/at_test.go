package collection

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestAtNext(t *testing.T) {
	Convey("At selects a configured index from each arriving collection", t, func() {
		op := NewAt[float64](1)
		out := tests.CollectSeq(op.Next(transport.Values([]float64{3, 4, 5})))

		So(out, ShouldResemble, []float64{4})
		So(op.Error(), ShouldBeNil)
	})

	Convey("At records a shape error for an index outside the collection", t, func() {
		op := NewAt[float64](3)
		out := tests.CollectSeq(op.Next(transport.Values([]float64{3, 4, 5})))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}

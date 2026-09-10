package store

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestHasNext(t *testing.T) {
	Convey("Has reports membership of a configured key", t, func() {
		op := NewHas[string, float64]("x")
		out := tests.CollectSeq(op.Next(transport.Values(
			map[string]float64{"x": 0},
			map[string]float64{},
		)))

		So(out, ShouldResemble, []bool{true, false})
		So(op.Error(), ShouldBeNil)
	})
}

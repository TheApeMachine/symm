package store

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRetainedNext(t *testing.T) {
	Convey("Retained holds the latest arrival and remains readable after the run", t, func() {
		memory := NewRetained(10.0)
		subtraction := arithmetic.NewSubtract[float64](memory.Read())

		out := tests.CollectSeq(subtraction.Next(transport.Values(3.0)))
		So(out[0], ShouldEqual, 7)

		tests.CollectSeq(memory.Next(transport.Values(20.0)))
		So(memory.Read(), ShouldEqual, 20)
	})
}

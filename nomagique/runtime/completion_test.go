package runtime

import (
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCompletionClone(t *testing.T) {
	Convey("An observation owns its results after the producing RPC is released", t, func() {
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		original, err := NewRootCompletion(segment)
		So(err, ShouldBeNil)
		outputs, err := original.NewOutputs(1)
		So(err, ShouldBeNil)
		So(outputs.At(0).SetNode("metric"), ShouldBeNil)
		value, err := capnp.NewText(segment, "observation N")
		So(err, ShouldBeNil)
		So(outputs.At(0).SetValue(value.ToPtr()), ShouldBeNil)
		retained, err := original.Clone()
		So(err, ShouldBeNil)
		defer retained.Message().Release()
		So(outputs.At(0).SetNode("reused slot"), ShouldBeNil)
		message.Release()
		copied, err := retained.Outputs()
		So(err, ShouldBeNil)
		name, err := copied.At(0).Node()
		So(err, ShouldBeNil)
		So(name, ShouldEqual, "metric")
		pointer, err := copied.At(0).Value()
		So(err, ShouldBeNil)
		So(pointer.Text(), ShouldEqual, "observation N")
	})
}

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

func TestCompletionAppend(t *testing.T) {
	Convey("A cycle owns ordered, individually stamped results after source release", t, func() {
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		defer message.Release()
		combined, err := NewRootCompletion(segment)
		So(err, ShouldBeNil)
		for sequence := range 3 {
			message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			source, err := NewRootCompletion(segment)
			So(err, ShouldBeNil)
			outputs, err := source.NewOutputs(1)
			So(err, ShouldBeNil)
			outputs.At(0).SetEpoch(77)
			outputs.At(0).SetSequence(int64(sequence))
			So(outputs.At(0).SetNode("source"), ShouldBeNil)
			So(combined.Append(source), ShouldBeNil)
			message.Release()
		}
		outputs, err := combined.Outputs()
		So(err, ShouldBeNil)
		So(outputs.Len(), ShouldEqual, 3)
		for index := range outputs.Len() {
			So(outputs.At(index).Epoch(), ShouldEqual, 77)
			So(outputs.At(index).Sequence(), ShouldEqual, index)
			name, err := outputs.At(index).Node()
			So(err, ShouldBeNil)
			So(name, ShouldEqual, "source")
		}
	})
}

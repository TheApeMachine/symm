package compiler

import (
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/ui"
)

func TestProgramBindings(t *testing.T) {
	Convey("Given a producer bound to a component port", t, func() {
		graph, err := ParseGraph([]byte(`{
			"id": "screen",
			"nodes": {
				"source": {
					"id": "source", "type": "store.Constant",
					"inputData": {"data": {"value": "{\"a\":1}"}},
					"connections": {"inputs": {}, "outputs": {"out": [{"nodeId": "label", "portName": "value"}]}}
				},
				"label": {
					"id": "label", "type": "ui.Text",
					"connections": {"inputs": {"value": [{"nodeId": "source", "portName": "out"}]}, "outputs": {}}
				}
			}
		}`))
		So(err, ShouldBeNil)
		program, err := Compile(graph, DefaultRegistry(), NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()

		publish := func() []string {
			So(program.Execute(context.Background(), nil), ShouldBeNil)
			frame, err := program.bindings()
			So(err, ShouldBeNil)

			if frame == nil {
				return nil
			}

			message, err := capnp.Unmarshal(frame)
			So(err, ShouldBeNil)
			root, err := ui.ReadRootBindings(message)
			So(err, ShouldBeNil)
			values, err := root.Values()
			So(err, ShouldBeNil)

			told := []string{}

			for index := range values.Len() {
				bound := values.At(index)
				component, err := bound.Component()
				So(err, ShouldBeNil)
				prop, err := bound.Prop()
				So(err, ShouldBeNil)
				value, err := bound.Value()
				So(err, ShouldBeNil)
				told = append(told, component+"."+prop+"="+value)
			}

			return told
		}

		Convey("The value is delivered once, and not again while it stands", func() {
			So(publish(), ShouldResemble, []string{`label.value={"a":1}`})
			So(publish(), ShouldBeNil)

		})
	})
}

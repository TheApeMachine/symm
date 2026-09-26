package compiler

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEmitDefinitionPorts(t *testing.T) {
	Convey("Given a definition with no ports", t, func() {
		var output strings.Builder
		emitDefinitionPorts(&output, "inputs", nil)
		So(output.String(), ShouldContainSubstring, "inputs: (_ports)")

		Convey("Then adding a port generates a used builder argument", func() {
			output.Reset()
			emitDefinitionPorts(&output, "inputs", []definitionPort{{Name: "value", Type: "Float64"}})
			So(output.String(), ShouldContainSubstring, "inputs: (ports)")
			So(output.String(), ShouldContainSubstring, `name: "value"`)
		})
	})
}

func BenchmarkEmitDefinitionPorts(b *testing.B) {
	ports := []definitionPort{{Name: "value", Type: "Float64"}}
	b.ReportAllocs()

	for b.Loop() {
		var output strings.Builder
		emitDefinitionPorts(&output, "inputs", ports)
	}
}

func TestEmitUIComponentNodeTypes(t *testing.T) {
	Convey("Given reflected structured visualization props", t, func() {
		var output strings.Builder
		emitUIComponentNodeTypes(&output, map[string]UIComponentMetadata{
			"ImpulseMap": {Name: "ImpulseMap", Props: []UIPropMetadata{{Name: "points", Type: "data"}}},
		})
		So(output.String(), ShouldContainSubstring, `ports.data({ name: "points", label: "points" })`)
		So(output.String(), ShouldNotContainSubstring, `ports.float64`)
	})
}

func TestEmitDefinitionNodeTypes(t *testing.T) {
	Convey("Authored graph factories remain visible capability nodes in the editor", t, func() {
		var output strings.Builder
		So(emitDefinitionNodeTypes(&output, nil), ShouldBeNil)
		So(output.String(), ShouldContainSubstring, `type: "factory:live_level3_shard"`)
		So(output.String(), ShouldContainSubstring, `label: "live_level3_shard factory"`)
		So(output.String(), ShouldContainSubstring, `ports.string({ name: "producer", label: "producer" })`)
		So(output.String(), ShouldContainSubstring, `ports.string({ name: "field", label: "field" })`)
		So(output.String(), ShouldContainSubstring, `ports.Capability({ name: "self", label: "self" })`)
	})
}

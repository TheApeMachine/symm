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

package compiler

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestReflectPorts(t *testing.T) {
	Convey("A configured Group also exposes its own capability for Workspace wiring", t, func() {
		_, outputs, _, found := reflectPorts(runtime.Group_TypeID)
		So(found, ShouldBeTrue)
		capabilities := 0

		for _, port := range outputs {
			if port.Name == "self" {
				So(port.Type, ShouldEqual, "Capability")
				capabilities++
			}
		}
		So(capabilities, ShouldEqual, 1)
	})
	Convey("Given a capability-only Transform resource", t, func() {
		inputs, outputs, _, found := reflectPorts(data.Transform_TypeID)
		So(found, ShouldBeTrue)
		So(inputs, ShouldBeEmpty)
		So(outputs, ShouldHaveLength, 1)
		So(outputs[0].Name, ShouldEqual, "self")
		So(outputs[0].Type, ShouldEqual, "Capability")
	})
}

func TestPortsOf(t *testing.T) {
	Convey("Given list-valued Grid output and gathered inputs", t, func() {
		reflected, err := ReflectInterface(store.Grid_TypeID)
		So(err, ShouldBeNil)
		outputs := portsOf(reflected.Outputs, false)
		for _, port := range outputs {
			if port.Name == "values" || port.Name == "present" {
				So(port.Type, ShouldEqual, "Structured")
			}
		}
		inputs := portsOf(reflected.Inputs, true)
		for _, port := range inputs {
			if port.Name == "metrics" {
				So(port.Type, ShouldEqual, "FanIn:Float64")
			}
		}
	})
}

package runtime

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSystemFail(t *testing.T) {
	Convey("The composed runtime owner retains and exposes stage failure", t, func() {
		system := NewSystem(t.Context(), "agent")
		system.Transition(READY)
		err := errors.New("execution acceptance unknown")
		system.Error(err)
		So(system.Error(), ShouldEqual, err)
		So(system.Status(), ShouldEqual, ERROR)
		system.Error(nil)
		So(system.Error(), ShouldEqual, err)
		So(system.Status(), ShouldEqual, ERROR)
		So(system.Close(), ShouldNotBeNil)
	})
}

func TestSystemFatal(t *testing.T) {
	Convey("A fatal system retains fatal status and error querying does not downgrade", t, func() {
		system := NewSystem(t.Context(), "category")
		system.Transition(READY)
		err := errors.New("measurement invalid")
		system.Error(err)
		So(system.Status(), ShouldEqual, ERROR)
		system.Transition(FATAL)
		So(system.Status(), ShouldEqual, FATAL)

		So(system.Error(), ShouldEqual, err)
		So(system.Status(), ShouldEqual, FATAL)

		err2 := errors.New("subsequent invalid")
		system.Error(err2)
		So(system.Status(), ShouldEqual, FATAL)
	})
}

func TestWorkspaceStatus(t *testing.T) {
	Convey("Given a freshly initialized workspace with stages", t, func() {
		node := &countingNode{}
		workspace := NewWorkspace(
			t.Context(), "test-workspace", [][]Node[*data.Measurement[float64]]{{node}}, nil,
		)
		defer workspace.Close()

		So(workspace.Status(), ShouldEqual, READY)
	})

	Convey("Given a workspace with no handlers", t, func() {
		workspace := NewWorkspace[int](t.Context(), "test-workspace", nil, nil)
		defer workspace.Close()

		So(workspace.Status(), ShouldEqual, ERROR)
	})
}

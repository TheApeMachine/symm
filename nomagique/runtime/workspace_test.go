package runtime

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestNewWorkspace(t *testing.T) {
	Convey("Given a freshly initialized workspace with stages", t, func() {
		node := &countingNode{}
		workspace := NewWorkspace(
			t.Context(), "test-workspace", [][]Node[*data.Measurement[float64]]{{node}}, nil,
		)
		defer func() { So(workspace.Close(), ShouldBeNil) }()

		So(workspace.Status(), ShouldEqual, INIT)
		// Idle admission must not touch the ring at all.
		channel := workspace.channel
		workspace.channel = nil
		workspace.Step(nil)
		workspace.channel = channel
		So(node.steps, ShouldEqual, 0)
		So(workspace.consumers[0].Status(), ShouldEqual, INIT)
		workspace.Transition(READY)
		So(workspace.Status(), ShouldEqual, READY)
		So(workspace.consumers[0].Status(), ShouldEqual, READY)
	})

	Convey("Given a workspace with no handlers", t, func() {
		workspace := NewWorkspace[int](t.Context(), "test-workspace", nil, nil)
		So(workspace, ShouldBeNil)
	})
}

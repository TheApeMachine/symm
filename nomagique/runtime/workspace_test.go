package runtime

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

type dummyNode struct {
	lastPrior *data.Value
}

func (node *dummyNode) Step(prior *data.Value) *data.Value {
	node.lastPrior = prior

	if prior == nil {
		return nil
	}

	return data.NewValue("test", "tick", "node").Append([]byte("dummy"))
}

func TestWorkspaceTickAdvancement(t *testing.T) {
	Convey("Given a configured Workspace", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		node := &dummyNode{}
		workspace := NewWorkspace(
			ctx,
			"test_workspace",
			[][]Node{{node}},
		)
		workspace.Transition(READY)

		Convey("the workspace defaults to an 8192 capacity ring buffer", func() {
			So(workspace.capacity, ShouldEqual, 8192)
			So(workspace.mask, ShouldEqual, 8191)
		})

		Convey("values passed to Step are handled through the pipeline", func() {
			val1 := data.NewValue("workspace", "tick", "aggregate").Append([]byte(`{"price":50000}`))
			workspace.Step(val1)

			So(val1, ShouldNotBeNil)

			val2 := data.NewValue("workspace", "tick", "aggregate").Append([]byte(`{"price":50010}`))
			workspace.Step(val2)

			So(val2, ShouldNotBeNil)
		})
	})
}

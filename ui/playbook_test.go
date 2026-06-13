package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func TestDecisionTreeWireFrame(t *testing.T) {
	Convey("Given the embedded playbook", t, func() {
		frame, err := DecisionTreeWireFrame()

		Convey("It should expose branches for the dashboard store", func() {
			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "decision_tree")

			branches, ok := frame["branches"].([]any)

			So(ok, ShouldBeTrue)
			So(len(branches), ShouldBeGreaterThan, 0)
		})
	})
}

func TestUiWireFrameTreePayload(t *testing.T) {
	Convey("Given a loaded playbook tree payload", t, func() {
		tree, err := logic.LoadTree()

		So(err, ShouldBeNil)

		frame, err := uiWireFrame(&qpool.QValue[any]{
			Type:  "decision_tree",
			Value: tree,
		}, "")

		Convey("It should preserve branches at the wire root", func() {
			So(err, ShouldBeNil)
			So(frame["type"], ShouldEqual, "decision_tree")

			branches, ok := frame["branches"].([]any)

			So(ok, ShouldBeTrue)
			So(len(branches), ShouldEqual, len(tree.Branches))
		})
	})
}

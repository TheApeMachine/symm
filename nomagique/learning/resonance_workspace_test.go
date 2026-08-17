package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewResonanceWorkspace(testingTB *testing.T) {
	Convey("Given an architecture and supervised target dimension", testingTB, func() {
		architecture := []int{3, 5, 2}
		workspace := newResonanceWorkspace(architecture, 2)

		Convey("It should allocate vectors at their layer dimensions", func() {
			So(workspace.xCol.Len(), ShouldEqual, architecture[0])
			So(workspace.grads[1].Len(), ShouldEqual, architecture[1])
			So(workspace.topPrior.Len(), ShouldEqual, architecture[2])
			So(workspace.taskPred.Len(), ShouldEqual, 2)
		})

		Convey("It should retain one singular-value slot per temporal dimension", func() {
			So(workspace.svdValues, ShouldHaveLength, architecture[2])
		})
	})
}

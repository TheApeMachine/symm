package runtime

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type workspaceTestNode struct {
	stepped chan int
}

func (node workspaceTestNode) Step(value int) int {
	node.stepped <- value

	return value
}

/*
TestWorkspaceAdmit verifies the subscription barrier used by root: pushes are
rejected while the universe is incomplete and flow immediately after admission.
*/
func TestWorkspaceAdmit(t *testing.T) {
	Convey("Given a waiting workload", t, func() {
		stepped := make(chan int, 1)
		workload := NewWorkload(
			t.Context(),
			[][]Node[int]{{workspaceTestNode{stepped: stepped}}},
		)
		defer workload.Close()
		workspace := NewWorkspace(t.Context(), []*Workload[int]{workload})
		defer workspace.Close()

		workload.Push(1)

		Convey("it rejects input before the subscription barrier", func() {
			select {
			case <-stepped:
				So("unexpected step", ShouldBeEmpty)
			default:
			}
		})

		Convey("admission opens the workload for streaming input", func() {
			workspace.Admit()
			workload.Push(2)

			select {
			case value := <-stepped:
				So(value, ShouldEqual, 2)
			case <-time.After(time.Second):
				So("workload did not step", ShouldBeEmpty)
			}
		})
	})
}

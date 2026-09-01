package runtime

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type workspaceProbe struct {
	steps     atomic.Uint32
	violation atomic.Bool
	done      chan uint32
	ingress   uint32
}

type workspaceJoinNode struct {
	mark uint32
	done bool
}

func (node workspaceJoinNode) Step(probe *workspaceProbe) *workspaceProbe {
	requires := probe.ingress

	if node.done {
		requires |= 8
	}

	if probe.steps.Load()&requires != requires {
		probe.violation.Store(true)
	}

	probe.steps.Or(node.mark)

	if node.done {
		probe.done <- probe.steps.Load()
	}

	return probe
}

type workspaceProbeNode struct {
	mark     uint32
	requires uint32
	done     bool
}

func (node workspaceProbeNode) Step(probe *workspaceProbe) *workspaceProbe {
	if probe.steps.Load()&node.requires != node.requires {
		probe.violation.Store(true)
	}

	probe.steps.Or(node.mark)

	if node.done {
		probe.done <- probe.steps.Load()
	}

	return probe
}

func TestNewWorkspace(t *testing.T) {
	Convey("Given a Workspace composed from nested Workload Nodes", t, func() {
		left := NewWorkload(t.Context(), "left", [][]Node[*workspaceProbe]{
			{workspaceProbeNode{mark: 1}},
			{workspaceProbeNode{mark: 2, requires: 1}},
		})
		right := NewWorkload(t.Context(), "right", [][]Node[*workspaceProbe]{
			{workspaceProbeNode{mark: 4}},
		})
		logic := NewWorkload(t.Context(), "logic", [][]Node[*workspaceProbe]{
			{workspaceJoinNode{mark: 8}},
		})
		strategy := NewWorkload(t.Context(), "strategy", [][]Node[*workspaceProbe]{
			{workspaceJoinNode{mark: 16, done: true}},
		})
		workspace := NewWorkspace(t.Context(), "workspace", [][]Node[*workspaceProbe]{
			{left, right},
			{logic},
			{strategy},
		})
		defer workspace.Close()

		So(workspace.Error(), ShouldBeNil)
		workspace.Admit()

		done := make(chan uint32, 2)
		leftProbe := &workspaceProbe{done: done, ingress: 1 | 2}
		rightProbe := &workspaceProbe{done: done, ingress: 4}
		left.Push(leftProbe)
		right.Push(rightProbe)

		for range 2 {
			select {
			case <-done:
			case <-time.After(time.Second):
				So("workspace did not complete its nested rings", ShouldBeEmpty)
			}
		}

		So(leftProbe.steps.Load(), ShouldEqual, uint32(1|2|8|16))
		So(rightProbe.steps.Load(), ShouldEqual, uint32(4|8|16))
		So(leftProbe.violation.Load(), ShouldBeFalse)
		So(rightProbe.violation.Load(), ShouldBeFalse)
	})

}

func TestWorkspaceAdmit(t *testing.T) {
	Convey("Given a Workspace awaiting the subscription barrier", t, func() {
		stepped := make(chan uint32, 1)
		workload := NewWorkload(t.Context(), "admit", [][]Node[*workspaceProbe]{
			{workspaceProbeNode{mark: 1, done: true}},
		})
		workspace := NewWorkspace(t.Context(), "workspace", [][]Node[*workspaceProbe]{{workload}})
		defer workspace.Close()
		probe := &workspaceProbe{done: stepped}

		workload.Push(probe)

		Convey("it rejects input before admission", func() {
			select {
			case <-stepped:
				So("unexpected step", ShouldBeEmpty)
			default:
			}
		})

		Convey("admission opens the outer and nested rings", func() {
			workspace.Admit()
			workload.Push(probe)

			select {
			case steps := <-stepped:
				So(steps, ShouldEqual, uint32(1))
			case <-time.After(time.Second):
				So("workspace did not step", ShouldBeEmpty)
			}
		})
	})
}

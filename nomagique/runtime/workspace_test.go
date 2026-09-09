package runtime

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type workspaceProbe struct {
	steps     atomic.Uint32
	visits    atomic.Int64
	violation atomic.Bool
	done      chan uint32
	ingress   uint32
}

type workspaceProbeNode struct {
	mark     uint32
	requires uint32
	done     bool
}

func (node workspaceProbeNode) Step(probe *workspaceProbe) *workspaceProbe {
	probe.visits.Add(1)
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
			{workspaceProbeNode{mark: 8, requires: 7}},
		})
		strategy := NewWorkload(t.Context(), "strategy", [][]Node[*workspaceProbe]{
			{workspaceProbeNode{mark: 16, requires: 15, done: true}},
		})
		workspace := NewWorkspace(t.Context(), "workspace", [][]Node[*workspaceProbe]{
			{left, right},
			{logic},
			{strategy},
		})
		defer func() {
			if err := workspace.Close(); err != nil {
				t.Error(err)
			}
		}()

		So(workspace.Error(), ShouldBeNil)
		workspace.Admit()

		done := make(chan uint32, 2)
		leftProbe := &workspaceProbe{done: done, ingress: 1 | 2}
		rightProbe := &workspaceProbe{done: done, ingress: 4}
		workspace.Push(leftProbe)
		workspace.Push(rightProbe)

		for range 2 {
			select {
			case <-done:
			case <-time.After(time.Second):
				So("workspace did not complete its nested rings", ShouldBeEmpty)
			}
		}

		So(leftProbe.steps.Load(), ShouldEqual, uint32(1|2|4|8|16))
		So(rightProbe.steps.Load(), ShouldEqual, uint32(1|2|4|8|16))
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
		defer func() {
			if err := workspace.Close(); err != nil {
				t.Error(err)
			}
		}()
		probe := &workspaceProbe{done: stepped}

		workspace.Push(probe)

		Convey("it rejects input before admission", func() {
			select {
			case <-stepped:
				So("unexpected step", ShouldBeEmpty)
			default:
			}
		})

		Convey("admission opens the outer and nested rings", func() {
			workspace.Admit()
			workspace.Push(probe)

			select {
			case steps := <-stepped:
				So(steps, ShouldEqual, uint32(1))
			case <-time.After(time.Second):
				So("workspace did not step", ShouldBeEmpty)
			}
		})
	})
}

func TestWorkspaceStep(t *testing.T) {
	Convey("Both nested workloads must finish before Grid starts", t, func() {
		leftEntered, rightEntered := make(chan struct{}), make(chan struct{})
		leftRelease, rightRelease := make(chan struct{}), make(chan struct{})
		left := NewWorkload(t.Context(), "left", [][]Node[int]{{workloadBarrierNode{leftEntered, leftRelease}}})
		right := NewWorkload(t.Context(), "right", [][]Node[int]{{workloadBarrierNode{rightEntered, rightRelease}}})
		count, done := &atomic.Int64{}, make(chan struct{})
		grid := NewWorkload(t.Context(), "grid", [][]Node[int]{{workloadCountNode{count, 1, done}}})
		workspace := NewWorkspace(t.Context(), "workspace", [][]Node[int]{{left, right}, {grid}})
		defer func() { So(workspace.Close(), ShouldBeNil) }()
		workspace.Admit()
		returned := make(chan struct{})
		go func() { workspace.Step(1); close(returned) }()
		<-leftEntered
		<-rightEntered
		So(count.Load(), ShouldEqual, 0)
		close(leftRelease)
		So(count.Load(), ShouldEqual, 0)
		close(rightRelease)
		<-returned
		So(count.Load(), ShouldEqual, 1)
	})
}

func BenchmarkWorkspaceStep(b *testing.B) {
	left := NewWorkload(b.Context(), "left", [][]Node[int]{{plainProbe{}}, {plainProbe{}}})
	right := NewWorkload(b.Context(), "right", [][]Node[int]{{plainProbe{}}, {plainProbe{}}})
	grid := NewWorkload(b.Context(), "grid", [][]Node[int]{{plainProbe{}}})
	workspace := NewWorkspace(b.Context(), "workspace", [][]Node[int]{{left, right}, {grid}})
	defer func() {
		if err := workspace.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	workspace.Admit()
	b.ReportAllocs()
	for b.Loop() {
		workspace.Step(1)
	}
}

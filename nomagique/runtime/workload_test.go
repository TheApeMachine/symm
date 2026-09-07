package runtime

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type workloadBarrierNode struct {
	entered chan struct{}
	release chan struct{}
}

func (node workloadBarrierNode) Step(value int) int {
	close(node.entered)
	<-node.release

	return value
}

type workloadCountNode struct {
	count  *atomic.Int64
	target int64
	done   chan struct{}
}

func (node workloadCountNode) Step(value int) int {
	if node.count.Add(1) == node.target {
		close(node.done)
	}

	return value
}

func TestWorkloadStep(t *testing.T) {
	Convey("Given a Workload used as a Node", t, func() {
		entered := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan struct{})
		workload := NewWorkload(t.Context(), "barrier", [][]Node[int]{
			{workloadBarrierNode{entered: entered, release: release}},
		})
		defer workload.Close()
		workload.admit()

		go func() {
			workload.Step(1)
			close(returned)
		}()
		<-entered

		Convey("Step does not return before the inner ring completes", func() {
			select {
			case <-returned:
				So("Step returned before completion", ShouldBeEmpty)
			default:
			}

			close(release)
			<-returned
		})
	})
}

/*
composedProbe records what the ring told it about its own position. Only the
ring knows both facts, so this is the whole contract: a node that asks gets
told, once, before anything is admitted.
*/
type composedProbe struct {
	group string
	stage int
	told  atomic.Int64
}

func (probe *composedProbe) Compose(group string, stage int) {
	probe.group = group
	probe.stage = stage
	probe.told.Add(1)
}

func (probe *composedProbe) Step(value int) int { return value }

type plainProbe struct{}

func (plainProbe) Step(value int) int { return value }

func TestWorkloadComposesItsNodes(t *testing.T) {
	Convey("Given a Workload built from staged nodes", t, func() {
		first := &composedProbe{}
		second := &composedProbe{}
		sibling := &composedProbe{}

		workload := NewWorkload(t.Context(), "ticker", [][]Node[int]{
			{first},
			{second, sibling, plainProbe{}},
		})
		defer workload.Close()

		Convey("Every node that asks is told the ring it runs in", func() {
			So(first.group, ShouldEqual, "ticker")
			So(second.group, ShouldEqual, "ticker")
			So(sibling.group, ShouldEqual, "ticker")
		})

		Convey("And which handler group it sits behind", func() {
			So(first.stage, ShouldEqual, 0)
			So(second.stage, ShouldEqual, 1)

			// Concurrent siblings share a stage: they run side by side against
			// the same value, which is exactly the fact nothing downstream can
			// recover from the order they report in.
			So(sibling.stage, ShouldEqual, second.stage)
		})

		Convey("And is told exactly once, before the ring is admitted", func() {
			So(first.told.Load(), ShouldEqual, int64(1))
			So(second.told.Load(), ShouldEqual, int64(1))
		})
	})
}

func TestWorkloadPush(t *testing.T) {
	Convey("Given concurrent writers wrapping the ring repeatedly", t, func() {
		const writers, events = 4, 1024
		count, done := &atomic.Int64{}, make(chan struct{})
		workload := newWorkload(t.Context(), "concurrent", [][]Node[int]{
			{workloadCountNode{count: count, target: writers * events, done: done}},
		}, writers)
		workload.admit()
		var producers sync.WaitGroup
		for writer := range writers {
			producers.Go(func() {
				for event := range events {
					workload.Push(writer*events + event)
				}
			})
		}
		producers.Wait()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("ring stopped making progress")
		}
		So(workload.Close(), ShouldBeNil)
		So(count.Load(), ShouldEqual, writers*events)
	})
}

func BenchmarkWorkloadPush(b *testing.B) {
	count := &atomic.Int64{}
	done := make(chan struct{})
	workload := NewWorkload(b.Context(), "push", [][]Node[int]{
		{workloadCountNode{count: count, target: int64(b.N), done: done}},
	})
	defer workload.Close()
	workload.admit()

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		workload.Push(index)
	}

	<-done
}

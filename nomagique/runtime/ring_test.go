package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smarty/go-disruptor"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestRingPublish(t *testing.T) {
	Convey("Given concurrent siblings and dependent stages across repeated ring wraps", t, func() {
		ring, err := newRing[*workspaceProbe](t.Context(), system.Cfg.Runtime.Workspace.Buffer)
		So(err, ShouldBeNil)
		ring.Start([][]disruptor.Handler{
			{NewConsumer(workspaceProbeNode{mark: 1}, ring.buffer, &ring.head), NewConsumer(workspaceProbeNode{mark: 2}, ring.buffer, &ring.head)},
			{NewConsumer(workspaceProbeNode{mark: 4, requires: 3}, ring.buffer, &ring.head)},
			{NewConsumer(workspaceProbeNode{mark: 8, requires: 7}, ring.buffer, &ring.head)},
		})
		// Four writers each wrap the configured ring sixteen times.
		const writers = 4
		events := int(system.Cfg.Runtime.Workspace.Buffer) * 16
		probes := make([]*workspaceProbe, writers*events)
		var producers sync.WaitGroup
		for writer := range writers {
			producers.Go(func() {
				for index := range events {
					probe := &workspaceProbe{}
					probes[writer*events+index] = probe
					if _, accepted := ring.Publish(probe); !accepted {
						panic("unexpected admission failure")
					}
				}
			})
		}
		producers.Wait()
		ring.Close()
		So(ring.completed.Load(), ShouldEqual, len(probes)-1)
		for _, probe := range probes {
			So(probe.steps.Load(), ShouldEqual, 15)
			So(probe.violation.Load(), ShouldBeFalse)
		}
	})

	Convey("Given a full ring blocked inside its consumer", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		entered, release := make(chan struct{}), make(chan struct{})
		ring, err := newRing[int](ctx, 1)
		So(err, ShouldBeNil)
		ring.Start([][]disruptor.Handler{{NewConsumer(workloadBarrierNode{entered: entered, release: release}, ring.buffer, &ring.head)}})
		_, accepted := ring.Publish(1)
		So(accepted, ShouldBeTrue)
		<-entered
		returned := make(chan bool, 1)
		go func() { _, accepted := ring.Publish(2); returned <- accepted }()
		cancel()
		select {
		case accepted := <-returned:
			So(accepted, ShouldBeFalse)
		case <-time.After(time.Second):
			t.Fatal("cancelled producer remained blocked")
		}
		close(release)
		ring.Close()
		So(ring.completed.Load(), ShouldEqual, 0)
	})
}

func TestRingClose(t *testing.T) {
	Convey("Given an idle ring with no committed events", t, func() {
		ring, err := newRing[int](t.Context(), 1)
		So(err, ShouldBeNil)
		ring.Start([][]disruptor.Handler{{NewConsumer(plainProbe{}, ring.buffer, &ring.head)}})
		done := make(chan struct{})
		go func() { ring.Close(); ring.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("idle readers failed to exit")
		}
		_, accepted := ring.Publish(1)
		So(accepted, ShouldBeFalse)
	})
}

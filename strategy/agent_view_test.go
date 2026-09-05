package strategy

import (
	"context"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestAgentSnapshot(t *testing.T) {
	Convey("Given the workspace as the sole state owner", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ctx.Err() == nil {
				agent.Step(nil)
				runtime.Gosched()
			}
		}()
		defer func() { cancel(); <-done }()

		Convey("Concurrent inspection receives an owned snapshot", func() {
			view, err := agent.Snapshot(ctx, "TEST/USD")
			So(err, ShouldBeNil)
			So(view.InitialCapital, ShouldEqual, agent.initial.String())
			So(view.Status, ShouldEqual, "waiting for market observations")
		})

		Convey("A cancelled reader never stalls the workspace", func() {
			request, stop := context.WithCancel(ctx)
			stop()
			_, err := agent.Snapshot(request, "TEST/USD")
			So(err, ShouldEqual, context.Canceled)
		})
	})
}

package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestLearningInspectorSnapshotCheckpoint(t *testing.T) {
	Convey("Model checkpoint requests are served by the workspace owner", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		done := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			defer close(done)
			for ctx.Err() == nil {
				agent.Step(nil)
			}
		}()
		requestCtx, requestCancel := context.WithTimeout(ctx, time.Second)
		checkpoint, err := agent.SnapshotCheckpoint(requestCtx)
		requestCancel()
		So(err, ShouldBeNil)
		So(checkpoint.Version, ShouldEqual, CheckpointVersion)
		cancel()
		<-done
		_, err = agent.SnapshotCheckpoint(ctx)
		So(err, ShouldNotBeNil)
	})
}

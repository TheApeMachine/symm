package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/logic"
)

func TestStoryShouldPublishUI(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a story publish interval", t, func() {
		viper.Set("system.audit.enabled", false)
		viper.Set("market.story.ui_interval", time.Millisecond)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "story-test")},
		)
		story := NewStory(ctx, pool)

		So(story, ShouldNotBeNil)
		So(subscriber, ShouldNotBeNil)

		drainStoryUI := func(count int) {
			for range count {
				frame, receiveErr := subscriber.Receive(internal.ChannelUI)

				So(receiveErr, ShouldBeNil)
				So(frame, ShouldNotBeNil)
			}
		}

		drainStoryUI(2)

		story.publishStoryUI(time.Now().Add(time.Second))

		statusFrame, statusErr := subscriber.Receive(internal.ChannelUI)

		Convey("It should publish story counters on the ui bus", func() {
			So(statusErr, ShouldBeNil)
			So(statusFrame, ShouldNotBeNil)
			So(statusFrame.Type, ShouldEqual, "story")

			statusPayload, statusOK := statusFrame.Value.(map[string]any)

			So(statusOK, ShouldBeTrue)
			So(statusPayload["story_ticks"], ShouldEqual, 0)
			So(statusPayload["playbook_evaluations"], ShouldEqual, 0)
		})

		treeFrame, treeErr := subscriber.Receive(internal.ChannelUI)

		Convey("It should publish the embedded playbook tree", func() {
			So(treeErr, ShouldBeNil)
			So(treeFrame, ShouldNotBeNil)
			So(treeFrame.Type, ShouldEqual, "decision_tree")

			payload, ok := treeFrame.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(payload["chart"], ShouldEqual, "decision_tree")

			branches, branchesOK := payload["branches"].([]*logic.Branch)

			So(branchesOK, ShouldBeTrue)
			So(len(branches), ShouldBeGreaterThan, 0)
		})

		Convey("It should increment counters as measurements are evaluated", func() {
			story.storyTicks = 12
			story.playbookEvaluations = 3
			story.publishStoryUI(time.Now().Add(2 * time.Second))

			nextStatusFrame, nextStatusErr := subscriber.Receive(internal.ChannelUI)

			So(nextStatusErr, ShouldBeNil)
			So(nextStatusFrame.Type, ShouldEqual, "story")

			nextStatusPayload, nextStatusOK := nextStatusFrame.Value.(map[string]any)

			So(nextStatusOK, ShouldBeTrue)
			So(nextStatusPayload["story_ticks"], ShouldEqual, 12)
			So(nextStatusPayload["playbook_evaluations"], ShouldEqual, 3)
		})
	})
}

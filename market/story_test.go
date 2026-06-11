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

		story.publishStoryUI(time.Now())

		frame, receiveErr := subscriber.Receive(internal.ChannelUI)

		Convey("It should publish the embedded playbook tree", func() {
			So(receiveErr, ShouldBeNil)
			So(frame, ShouldNotBeNil)

			payload, ok := frame.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(payload["chart"], ShouldEqual, "decision_tree")

			branches, branchesOK := payload["branches"].([]*logic.Branch)

			So(branchesOK, ShouldBeTrue)
			So(len(branches), ShouldBeGreaterThan, 0)
		})
	})
}

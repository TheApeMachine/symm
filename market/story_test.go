package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestStoryShouldPublishUI(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a story publish interval", t, func() {
		viper.Set("market.story.ui_interval", time.Millisecond)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		story := NewStory(ctx, pool)

		So(story, ShouldNotBeNil)
		So(story.playbookProbe, ShouldNotBeNil)

		story.publishStoryUI(time.Now())

		frame := story.playbookProbe.DecisionTreeFrame()

		Convey("It should publish playbook tree nodes", func() {
			So(frame["chart"], ShouldEqual, "decision_tree")
			So(len(frame["nodes"].([]map[string]any)), ShouldBeGreaterThan, 0)
		})
	})
}

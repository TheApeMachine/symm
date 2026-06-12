package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestNewStoryRejectsLiveWithoutCapitalProvider(t *testing.T) {
	testconfig.Load(t)
	t.Cleanup(func() {
		viper.Set("trading.model", "paper")
	})

	Convey("Given live trading without a live capital provider", t, func() {
		viper.Set("trading.model", "live")
		viper.Set("system.audit.enabled", false)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		story, err := NewStory(ctx, pool)

		Convey("It should fail during startup before entries are sized", func() {
			So(story, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "live capital provider not configured")
		})
	})
}

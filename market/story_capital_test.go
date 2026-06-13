package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/trader"
)

func TestNewStoryUsesLiveWalletCapitalProvider(t *testing.T) {
	testconfig.Load(t)
	t.Cleanup(func() {
		viper.Set("trading.model", "paper")
	})

	Convey("Given live trading with wallet-backed capital", t, func() {
		viper.Set("trading.model", "live")
		viper.Set("system.audit.enabled", false)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		story, err := NewStory(ctx, pool, NewTestTouchRegistry(t, ctx, pool))

		Convey("It should start with a wallet capital provider", func() {
			So(err, ShouldBeNil)
			So(story, ShouldNotBeNil)
			_, ok := story.capitalProvider.(*trader.WalletCapitalProvider)
			So(ok, ShouldBeTrue)
		})
	})
}

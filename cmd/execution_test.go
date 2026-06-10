package cmd

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/paper"
)

func TestWireExecutionAdapter(t *testing.T) {
	Convey("Given a configured trading model", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		bookStore := krakenmarket.NewBookStore(10)

		Convey("It should wire paper execution only for trading.model=paper", func() {
			viper.Set("trading.model", "paper")

			systems, err := WireExecutionAdapter(ctx, pool, bookStore)

			So(err, ShouldBeNil)
			So(len(systems), ShouldEqual, 1)
			So(systems[0], ShouldHaveSameTypeAs, paper.NewWebSocket(ctx, pool, bookStore))
		})

		Convey("It should fail closed for live without credentials", func() {
			viper.Set("trading.model", "live")
			t.Setenv("SYMM_KRAKEN_API_KEY", "")
			t.Setenv("SYMM_KRAKEN_API_SECRET", "")

			systems, err := WireExecutionAdapter(ctx, pool, bookStore)

			So(systems, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("It should reject unknown trading models", func() {
			viper.Set("trading.model", "simulation")

			systems, err := WireExecutionAdapter(ctx, pool, bookStore)

			So(systems, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

package manifold

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSystemIngestFuturesBook(t *testing.T) {
	convey.Convey("Given a manifold system with a registered perpetual", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		viper.Set("signals.manifold.measurements_capacity", 16)
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		system := NewSystem(ctx, pool)

		convey.Convey("It should feed futures product books into the shared field", func() {
			convey.So(system, convey.ShouldNotBeNil)

			perpIdentity, identityErr := krakenmarket.FuturesIdentityFromProduct("PI_XBTUSD")
			convey.So(identityErr, convey.ShouldBeNil)

			system.field.universe.loadIdentity(perpIdentity)

			handled, err := system.ingestFuturesBook(&krakenmarket.BookUpdate{
				Symbol:    "PI_XBTUSD",
				Timestamp: time.Now(),
				Type:      "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 100, Qty: 1},
					{Price: 99.99, Qty: 1},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 101.01, Qty: 1},
				},
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(handled, convey.ShouldBeTrue)

			system.Close()
		})

		convey.Convey("It should ignore spot pair books", func() {
			convey.So(system, convey.ShouldNotBeNil)

			handled, err := system.ingestFuturesBook(&krakenmarket.BookUpdate{
				Symbol:    "XBT/USD",
				Timestamp: time.Now(),
				Type:      "snapshot",
				Bids:      []krakenmarket.BookLevel{{Price: 100, Qty: 1}},
				Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
			})

			convey.So(err, convey.ShouldBeNil)
			convey.So(handled, convey.ShouldBeFalse)

			system.Close()
		})
	})
}

func TestDepositBookLevelPerpetualLane(t *testing.T) {
	convey.Convey("Given a perpetual instrument state", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()
		convey.So(err, convey.ShouldBeNil)

		perpIdentity, identityErr := krakenmarket.FuturesIdentityFromProduct("PI_XBTUSD")
		convey.So(identityErr, convey.ShouldBeNil)

		state := field.universe.loadIdentity(perpIdentity)
		state.bookReady = true
		state.midPrice = 100
		state.tickSize = 0.01
		state.book = krakenmarket.BookUpdate{
			Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 2}},
			Asks: []krakenmarket.BookLevel{{Price: 101, Qty: 2}},
		}

		convey.Convey("It should project deposits onto the perpetual Y lane", func() {
			coords := field.universe.coords(state, 0)

			convey.So(coords.cellY, convey.ShouldEqual, uint32(krakenmarket.InstrumentLanePerpetual))

			field.Close()
		})
	})
}

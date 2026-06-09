package manifold

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/physics"
)

func TestFieldFeedTradeWhaleParticle(t *testing.T) {
	convey.Convey("Given a manifold field", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()

		convey.Convey("It should enqueue whale trades as PIC particles instead of grid deposits", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			field.lastStepAt = time.Now()

			state := field.universe.loadSymbol("XBT/USD")
			state.midPrice = 50000
			state.bookReady = true
			state.tradeQtys = []float64{0.1, 0.2, 0.15, 0.12, 0.18}
			state.returns = []float64{0.01, -0.008, 0.012}

			smallErr := field.FeedTrade(&krakenmarket.TradeUpdate{
				Symbol: "XBT/USD",
				Price:  50010,
				Qty:    0.15,
				Side:   "buy",
			}, time.Now())

			convey.So(smallErr, convey.ShouldBeNil)
			convey.So(len(field.pendingDeposits), convey.ShouldEqual, 1)
			convey.So(len(field.pendingWhales), convey.ShouldEqual, 0)

			whaleErr := field.FeedTrade(&krakenmarket.TradeUpdate{
				Symbol: "XBT/USD",
				Price:  50010,
				Qty:    50,
				Side:   "buy",
			}, time.Now())

			convey.So(whaleErr, convey.ShouldBeNil)
			convey.So(len(field.pendingWhales), convey.ShouldEqual, 1)
			convey.So(field.pendingWhales[0].oscillator.VelX, convey.ShouldBeGreaterThan, 0)
			convey.So(field.pendingWhales[0].oscillator.PosY, convey.ShouldEqual, 0)

			field.Close()
		})
	})
}

func TestCapCarriers(t *testing.T) {
	convey.Convey("Given more carriers than max modes", t, func() {
		oscillators := make([]physics.Oscillator, 40)
		carriers := make([]fieldCarrier, 40)

		for index := range oscillators {
			heat := float64(index) * 0.01
			oscillators[index] = physics.Oscillator{Heat: heat}
			carriers[index] = fieldCarrier{
				role:       "symbol",
				symbol:     "SYM",
				oscillator: oscillators[index],
			}
		}

		convey.Convey("It should keep the hottest max modes", func() {
			trimmedOscillators, trimmedCarriers := capCarriers(oscillators, carriers, 32)

			convey.So(len(trimmedOscillators), convey.ShouldEqual, 32)
			convey.So(len(trimmedCarriers), convey.ShouldEqual, 32)
			convey.So(trimmedOscillators[0].Heat, convey.ShouldEqual, oscillators[39].Heat)
			convey.So(trimmedOscillators[31].Heat, convey.ShouldEqual, oscillators[8].Heat)
		})
	})
}

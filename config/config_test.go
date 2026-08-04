package config

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLoad(t *testing.T) {
	Convey("Given a normalized viper tree", t, func() {
		viper.Reset()
		viper.Set("system.actor.buffer", 64)
		viper.Set("system.websocket.channel.buffer", 128)
		viper.Set("ui.addr", "127.0.0.1:8765")
		viper.Set("trading.model", "paper")
		viper.Set("trading.allocation.max_fraction", 0.2)
		viper.Set("trading.slots.normal", 2)
		viper.Set("trading.slots.reserved", 2)
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 10)
		viper.Set("market.subscribe_pace", "20ms")
		viper.Set("market.baseline_halflife", "30s")
		viper.Set("trading.risk.max_loss_fraction", 0.01)
		viper.Set("trading.risk.portfolio_loss_fraction", 0.03)
		viper.Set("trading.risk.noise_multiple", 3.0)
		viper.Set("trading.risk.trail_multiple", 2.0)
		viper.Set("trading.risk.arm_multiple", 2.0)
		viper.Set("trading.risk.lock_multiple", 1.0)
		viper.Set("trading.risk.min_edge_multiple", 1.0)
		viper.Set("trading.risk.min_ticks", 4)
		viper.Set("trading.risk.confirm_marks", 3)

		Convey("When Load snapshots config", func() {
			cfg, err := Load()

			Convey("It returns an immutable snapshot", func() {
				So(err, ShouldBeNil)
				So(cfg.System.ActorBuffer, ShouldEqual, 64)
				So(cfg.System.ChannelBuffer, ShouldEqual, 128)
				So(cfg.UI.Addr, ShouldEqual, "127.0.0.1:8765")
				So(cfg.Trading.Model, ShouldEqual, "paper")
			})
		})

		Convey("When system.actor.buffer is non-positive", func() {
			viper.Set("system.actor.buffer", 0)
			_, err := Load()

			Convey("It rejects the configuration", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When system.websocket.channel.buffer is non-positive", func() {
			viper.Set("system.websocket.channel.buffer", 0)
			_, err := Load()

			Convey("It rejects the configuration", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When ui.addr is empty", func() {
			viper.Set("ui.addr", "")
			_, err := Load()

			Convey("It rejects the configuration", func() {
				So(err, ShouldNotBeNil)
			})
		})

		/*
			Each of these degenerates silently rather than erroring at the point
			of use, which is why they are refused at boot. A loss fraction of
			zero does not disable the position cap, it makes the whole wallet one
			lot's budget; an arm buffer at or below the lock buffer lets profit
			protection arm and fire on the same tick; a confirmation count below
			one turns every wick into an exit.
		*/
		Convey("When the risk geometry would not protect a position", func() {
			for key, value := range map[string]any{
				"trading.risk.max_loss_fraction":       0.0,
				"trading.risk.portfolio_loss_fraction": 0.0,
				"trading.risk.noise_multiple":          0.0,
				"trading.risk.trail_multiple":          0.0,
				"trading.risk.lock_multiple":           0.0,
				"trading.risk.min_ticks":               0,
				"trading.risk.confirm_marks":           0,
			} {
				original := viper.Get(key)
				viper.Set(key, value)
				_, err := Load()

				So(err, ShouldNotBeNil)
				viper.Set(key, original)
			}
		})

		Convey("When arming is not separated from locking", func() {
			viper.Set("trading.risk.arm_multiple", 1.0)
			_, err := Load()

			So(err, ShouldNotBeNil)
		})

		Convey("When one position may lose more than the whole account", func() {
			viper.Set("trading.risk.max_loss_fraction", 0.5)
			viper.Set("trading.risk.portfolio_loss_fraction", 0.1)
			_, err := Load()

			So(err, ShouldNotBeNil)
		})
	})
}

func TestFixture(t *testing.T) {
	Convey("Given Fixture config", t, func() {
		cfg := Fixture()

		Convey("It supplies deterministic test defaults", func() {
			So(cfg.Market.QuoteCurrency, ShouldEqual, "USD")
			So(cfg.System.ActorBuffer, ShouldBeGreaterThan, 0)
			So(cfg.Trading.SlotsNormal, ShouldBeGreaterThan, 0)
			So(cfg.System.CheckpointEvery, ShouldBeGreaterThan, 0)
			So(cfg.Market.SubscribePace, ShouldBeGreaterThan, 0)
			So(cfg.Market.BaselineHalflife, ShouldBeGreaterThan, 0)
			So(cfg.Signals.FeedTimelineCapacity, ShouldBeGreaterThan, 0)
			So(cfg.Cognitive.TickBudget, ShouldBeGreaterThan, 0)
			So(cfg.Cognitive.InMemory, ShouldBeTrue)
		})
	})
}

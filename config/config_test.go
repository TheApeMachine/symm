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

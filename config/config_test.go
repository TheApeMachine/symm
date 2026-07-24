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
	})
}

func TestFixture(t *testing.T) {
	Convey("Given Fixture config", t, func() {
		cfg := Fixture()

		Convey("It supplies deterministic test defaults", func() {
			So(cfg.Market.QuoteCurrency, ShouldEqual, "USD")
			So(cfg.System.ActorBuffer, ShouldBeGreaterThan, 0)
			So(cfg.Trading.SlotsNormal, ShouldBeGreaterThan, 0)
		})
	})
}

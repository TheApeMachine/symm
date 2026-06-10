package config

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLoadTradingConfig(t *testing.T) {
	Convey("Given valid trading config", t, func() {
		viper.Set("trading.model", "paper")
		viper.Set("trading.position_fraction", 0.2)
		viper.Set("trading.max_concurrent_positions", 16)
		viper.Set("trading.max_quote_age", 15*time.Second)
		viper.Set("trading.max_spread_bps", 120.0)
		viper.Set("trading.max_slippage_bps", 80.0)
		viper.Set("trading.order_ack_timeout", 500*time.Millisecond)
		viper.Set("trading.entry.transit_ttl", 5*time.Second)

		config, err := LoadTradingConfig()

		Convey("It should load and validate", func() {
			So(err, ShouldBeNil)
			So(config.Model, ShouldEqual, "paper")
			So(config.MaxQuoteAge, ShouldEqual, 15*time.Second)
		})
	})
}

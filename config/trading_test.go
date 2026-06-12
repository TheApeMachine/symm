package config

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLoadTradingConfig(t *testing.T) {
	Convey("Given valid trading config", t, func() {
		setValidTradingConfig()

		config, err := LoadTradingConfig()

		Convey("It should load and validate", func() {
			So(err, ShouldBeNil)
			So(config.Model, ShouldEqual, "paper")
			So(config.MaxQuoteAge, ShouldEqual, 15*time.Second)
		})
	})
}

func TestTradingConfigValidationRejectsUnsafeRisk(t *testing.T) {
	Convey("Given an oversized position fraction", t, func() {
		setValidTradingConfig()
		viper.Set("trading.position_fraction", 1.2)

		_, err := LoadTradingConfig()

		So(err, ShouldNotBeNil)
	})

	Convey("Given a secondary slot larger than position fraction", t, func() {
		setValidTradingConfig()
		viper.Set("trading.entry.secondary_slot_fraction", 0.3)

		_, err := LoadTradingConfig()

		So(err, ShouldNotBeNil)
	})

	Convey("Given an impossible spread limit", t, func() {
		setValidTradingConfig()
		viper.Set("trading.max_spread_bps", 10001.0)

		_, err := LoadTradingConfig()

		So(err, ShouldNotBeNil)
	})
}

func setValidTradingConfig() {
	viper.Reset()
	viper.Set("trading.model", "paper")
	viper.Set("trading.position_fraction", 0.2)
	viper.Set("trading.max_concurrent_positions", 5)
	viper.Set("trading.entry.primary_slot_count", 2)
	viper.Set("trading.entry.secondary_slot_fraction", 0.1)
	viper.Set("trading.max_quote_age", 15*time.Second)
	viper.Set("trading.max_spread_bps", 120.0)
	viper.Set("trading.max_slippage_bps", 80.0)
	viper.Set("trading.order_ack_timeout", 500*time.Millisecond)
	viper.Set("trading.entry.transit_ttl", 5*time.Second)
}

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

func TestTradingConfigValidationRejectsInvalidModel(t *testing.T) {
	Convey("Given an unsupported trading model", t, func() {
		setValidTradingConfig()
		viper.Set("trading.model", "invalid")

		_, err := LoadTradingConfig()

		So(err, ShouldNotBeNil)
	})
}

func setValidTradingConfig() {
	viper.Reset()
	viper.Set("trading.model", "paper")
	viper.Set("trading.max_concurrent_positions", 4)
	viper.Set("trading.entry.opportunity_slot_count", 2)
	viper.Set("trading.max_quote_age", 15*time.Second)
	viper.Set("trading.order_ack_timeout", 500*time.Millisecond)
	viper.Set("trading.entry.transit_ttl", 5*time.Second)
}

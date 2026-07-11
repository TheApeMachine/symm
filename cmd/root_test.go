package cmd

import (
	"testing"

	"github.com/spf13/viper"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateTradingModel(t *testing.T) {
	Convey("Given the remediation trading lock", t, func() {
		previous := viper.GetString("trading.model")
		defer viper.Set("trading.model", previous)

		Convey("Paper mode is accepted", func() {
			viper.Set("trading.model", "paper")
			So(validateTradingModel(), ShouldBeNil)
		})

		Convey("Live mode is rejected", func() {
			viper.Set("trading.model", "live")
			So(validateTradingModel(), ShouldNotBeNil)
		})

		Convey("Unknown mode is rejected", func() {
			viper.Set("trading.model", "typo")
			So(validateTradingModel(), ShouldNotBeNil)
		})
	})
}

func TestLiveReadiness(t *testing.T) {
	Convey("Given the live.* confirmation and limit gates", t, func() {
		keys := []string{
			"live.confirm",
			"live.api_key_permissions_confirmed",
			"live.clock_synchronized",
			"live.exchange_connectivity_confirmed",
			"live.paper_live_parity_passed",
			"live.native_protective_stops_supported",
			"live.max_order_notional",
			"live.max_daily_loss",
		}
		previous := make(map[string]any, len(keys))

		for _, key := range keys {
			previous[key] = viper.Get(key)
		}

		defer func() {
			for key, value := range previous {
				viper.Set(key, value)
			}
		}()

		Convey("A fully unconfirmed, unconfigured state reports every gate unmet", func() {
			for _, key := range keys {
				viper.Set(key, nil)
			}

			reasons := liveReadiness()

			So(len(reasons), ShouldEqual, 8)
		})

		Convey("Confirming every flag and configuring positive limits clears all gates", func() {
			viper.Set("live.confirm", "I UNDERSTAND THE RISK")
			viper.Set("live.api_key_permissions_confirmed", true)
			viper.Set("live.clock_synchronized", true)
			viper.Set("live.exchange_connectivity_confirmed", true)
			viper.Set("live.paper_live_parity_passed", true)
			viper.Set("live.native_protective_stops_supported", true)
			viper.Set("live.max_order_notional", 50)
			viper.Set("live.max_daily_loss", 20)

			So(liveReadiness(), ShouldBeEmpty)
		})

		Convey("A single unmet confirmation is reported on its own", func() {
			viper.Set("live.confirm", "I UNDERSTAND THE RISK")
			viper.Set("live.api_key_permissions_confirmed", true)
			viper.Set("live.clock_synchronized", true)
			viper.Set("live.exchange_connectivity_confirmed", true)
			viper.Set("live.paper_live_parity_passed", true)
			viper.Set("live.native_protective_stops_supported", false)
			viper.Set("live.max_order_notional", 50)
			viper.Set("live.max_daily_loss", 20)

			reasons := liveReadiness()

			So(len(reasons), ShouldEqual, 1)
			So(reasons[0], ShouldContainSubstring, "native_protective_stops_supported")
		})

		Convey("A zero or negative risk limit is reported as unmet", func() {
			viper.Set("live.confirm", "I UNDERSTAND THE RISK")
			viper.Set("live.api_key_permissions_confirmed", true)
			viper.Set("live.clock_synchronized", true)
			viper.Set("live.exchange_connectivity_confirmed", true)
			viper.Set("live.paper_live_parity_passed", true)
			viper.Set("live.native_protective_stops_supported", true)
			viper.Set("live.max_order_notional", 0)
			viper.Set("live.max_daily_loss", -5)

			reasons := liveReadiness()

			So(len(reasons), ShouldEqual, 2)
		})
	})
}

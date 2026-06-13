package testconfig

import (
	"time"

	"github.com/spf13/viper"
)

/*
SeedRegimeDefaults sets the minimum operational inputs that derived regime sizing
needs. Signal construction calls market.MustSignalMeasurementCapacity, which
derives capacity from gauge cadence and the symbol universe.
*/
func SeedRegimeDefaults() {
	if viper.GetDuration("telemetry.gauge.publish_interval") <= 0 {
		viper.Set("telemetry.gauge.publish_interval", "100ms")
	}

	if len(viper.GetStringSlice("market.default_symbols")) == 0 {
		viper.Set("market.default_symbols", []string{"BTC/USD"})
	}

	if viper.GetInt("market.book_depth_levels") <= 0 {
		viper.Set("market.book_depth_levels", 10)
	}
}

/*
SeedCompactRegime sets operational inputs that yield a small derived regime window
for unit tests that need bounded warmup and return history.
*/
func SeedCompactRegime() {
	viper.Set("telemetry.gauge.publish_interval", 60*time.Second)
	viper.Set("market.default_symbols", []string{"BTC/USD"})
	viper.Set("market.book_depth_levels", 10)
}

package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoadThresholdConfigValidatesUnitRanges(test *testing.T) {
	viper.Reset()
	viper.Set("trading.entry.confidence_baseline", 1.2)

	_, thresholdErr := LoadThresholdConfig()

	if thresholdErr == nil {
		test.Fatal("expected entry confidence validation error")
	}
}

func TestLoadThresholdConfigRejectsFloorAboveBaseline(test *testing.T) {
	viper.Reset()
	viper.Set("trading.entry.confidence_baseline", 0.6)
	viper.Set("trading.exit.confidence_baseline", 0.5)
	viper.Set("trading.exit.confidence_floor", 0.55)

	_, thresholdErr := LoadThresholdConfig()

	if thresholdErr == nil {
		test.Fatal("expected exit floor validation error")
	}
}

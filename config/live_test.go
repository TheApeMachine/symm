package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestCheckLiveReadinessBypassesPaper(test *testing.T) {
	err := CheckLiveReadiness(
		TradingConfig{Model: "paper"},
		LiveReadinessConfig{},
		LiveReadinessDependencies{AuditErr: errors.New("audit down")},
	)

	if err != nil {
		test.Fatalf("paper mode should bypass live readiness, got %v", err)
	}
}

func TestCheckLiveReadinessRejectsUnsafeLive(test *testing.T) {
	err := CheckLiveReadiness(
		TradingConfig{Model: "live"},
		LiveReadinessConfig{},
		LiveReadinessDependencies{},
	)

	if err == nil {
		test.Fatal("expected live readiness error")
	}

	errText := err.Error()

	for _, expected := range []string{
		"live.confirm",
		"SYMM_KRAKEN_API_KEY",
		"SYMM_KRAKEN_API_SECRET",
		"live.api_key_permissions_confirmed",
		"live.max_order_notional",
		"live.max_daily_loss",
		"live.clock_synchronized",
		"live.exchange_connectivity_confirmed",
		"live.paper_live_parity_passed",
		"trading.max_concurrent_positions",
	} {
		if !strings.Contains(errText, expected) {
			test.Fatalf("expected %q in readiness error %q", expected, errText)
		}
	}
}

func TestCheckLiveReadinessAcceptsConfirmedLive(test *testing.T) {
	err := CheckLiveReadiness(
		TradingConfig{
			Model:                  "live",
			MaxConcurrentPositions: 1,
		},
		LiveReadinessConfig{
			Confirm:                       LiveConfirmPhrase,
			APIKeyPermissionsConfirmed:    true,
			ClockSynchronized:             true,
			ExchangeConnectivityConfirmed: true,
			PaperLiveParityPassed:         true,
			MaxOrderNotional:              100,
			MaxDailyLoss:                  25,
		},
		LiveReadinessDependencies{
			APIKey:    "key",
			APISecret: "secret",
		},
	)

	if err != nil {
		test.Fatalf("expected confirmed live readiness, got %v", err)
	}
}

func TestLoadLiveReadinessConfig(test *testing.T) {
	viper.Reset()
	viper.Set("live.confirm", LiveConfirmPhrase)
	viper.Set("live.api_key_permissions_confirmed", true)
	viper.Set("live.clock_synchronized", true)
	viper.Set("live.exchange_connectivity_confirmed", true)
	viper.Set("live.paper_live_parity_passed", true)
	viper.Set("live.max_order_notional", 100.0)
	viper.Set("live.max_daily_loss", 25.0)

	config := LoadLiveReadinessConfig()

	if config.Confirm != LiveConfirmPhrase {
		test.Fatalf("unexpected confirm phrase: %q", config.Confirm)
	}

	if config.MaxOrderNotional != 100 {
		test.Fatalf("unexpected max order notional: %f", config.MaxOrderNotional)
	}
}

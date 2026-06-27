package live

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func resetLiveTest(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Setenv("SYMM_LIVE", "")
	t.Setenv("SYMM_KRAKEN_API_KEY", "")
	t.Setenv("SYMM_KRAKEN_API_SECRET", "")
}

func configureReadyLive(t *testing.T) {
	t.Helper()
	viper.Set("trading.model", "live")
	viper.Set("trading.margin_enabled", false)
	viper.Set("live.confirm", ConfirmationPhrase)
	viper.Set("live.api_key_permissions_confirmed", true)
	viper.Set("live.clock_synchronized", true)
	viper.Set("live.exchange_connectivity_confirmed", true)
	viper.Set("live.paper_live_parity_passed", true)
	viper.Set("live.max_order_notional", 100)
	viper.Set("live.max_daily_loss", 25)
	t.Setenv("SYMM_KRAKEN_API_KEY", "key")
	t.Setenv("SYMM_KRAKEN_API_SECRET", "secret")
}

func TestValidateReadinessAllowsPaperMode(t *testing.T) {
	resetLiveTest(t)
	viper.Set("trading.model", "paper")

	if err := ValidateReadiness(); err != nil {
		t.Fatalf("paper readiness returned error: %v", err)
	}
}

func TestValidateReadinessBlocksLiveWithoutNativeProtectiveStops(t *testing.T) {
	resetLiveTest(t)
	configureReadyLive(t)

	err := ValidateReadiness()
	if err == nil {
		t.Fatal("expected live mode to be blocked without native protective stops")
	}
	if !strings.Contains(err.Error(), NativeProtectiveStopsRequired) {
		t.Fatalf("readiness error %q does not mention native protective stops", err.Error())
	}
}

func TestValidateReadinessBlocksExplicitLiveEnvWithoutNativeProtectiveStops(t *testing.T) {
	resetLiveTest(t)
	configureReadyLive(t)
	viper.Set("trading.model", "paper")
	t.Setenv("SYMM_LIVE", "true")

	err := ValidateReadiness()
	if err == nil {
		t.Fatal("expected explicit live env to be blocked without native protective stops")
	}
	if !strings.Contains(err.Error(), NativeProtectiveStopsRequired) {
		t.Fatalf("readiness error %q does not mention native protective stops", err.Error())
	}
}

func TestValidateReadinessReportsMissingSafetyFields(t *testing.T) {
	cases := []struct {
		name string
		key  string
		mut  func(*testing.T)
	}{
		{
			name: "confirm",
			key:  "live.confirm",
			mut:  func(t *testing.T) { viper.Set("live.confirm", "") },
		},
		{
			name: "permissions",
			key:  "live.api_key_permissions_confirmed",
			mut:  func(t *testing.T) { viper.Set("live.api_key_permissions_confirmed", false) },
		},
		{
			name: "clock",
			key:  "live.clock_synchronized",
			mut:  func(t *testing.T) { viper.Set("live.clock_synchronized", false) },
		},
		{
			name: "connectivity",
			key:  "live.exchange_connectivity_confirmed",
			mut:  func(t *testing.T) { viper.Set("live.exchange_connectivity_confirmed", false) },
		},
		{
			name: "parity",
			key:  "live.paper_live_parity_passed",
			mut:  func(t *testing.T) { viper.Set("live.paper_live_parity_passed", false) },
		},
		{
			name: "max_order",
			key:  "live.max_order_notional",
			mut:  func(t *testing.T) { viper.Set("live.max_order_notional", 0) },
		},
		{
			name: "max_loss",
			key:  "live.max_daily_loss",
			mut:  func(t *testing.T) { viper.Set("live.max_daily_loss", 0) },
		},
		{
			name: "api_key",
			key:  "SYMM_KRAKEN_API_KEY",
			mut:  func(t *testing.T) { t.Setenv("SYMM_KRAKEN_API_KEY", "") },
		},
		{
			name: "api_secret",
			key:  "SYMM_KRAKEN_API_SECRET",
			mut:  func(t *testing.T) { t.Setenv("SYMM_KRAKEN_API_SECRET", "") },
		},
		{
			name: "margin",
			key:  "trading.margin_enabled",
			mut:  func(t *testing.T) { viper.Set("trading.margin_enabled", true) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetLiveTest(t)
			configureReadyLive(t)
			tc.mut(t)

			err := ValidateReadiness()
			if err == nil {
				t.Fatalf("expected readiness failure for %s", tc.key)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("readiness error %q does not mention %s", err.Error(), tc.key)
			}
		})
	}
}

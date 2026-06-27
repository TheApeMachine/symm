package live

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

const ConfirmationPhrase = "I_UNDERSTAND_THIS_PLACES_REAL_ORDERS"
const NativeProtectiveStopsRequired = "live trading requires native protective stops"

func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SYMM_LIVE"))) {
	case "1", "true", "yes", "live":
		return true
	}

	return strings.EqualFold(strings.TrimSpace(viper.GetString("trading.model")), "live")
}

func ValidateReadiness() error {
	if !Enabled() {
		return nil
	}

	failures := make([]string, 0)

	if strings.TrimSpace(viper.GetString("live.confirm")) != ConfirmationPhrase {
		failures = append(failures, "live.confirm")
	}
	for _, key := range []string{
		"live.api_key_permissions_confirmed",
		"live.clock_synchronized",
		"live.exchange_connectivity_confirmed",
		"live.paper_live_parity_passed",
	} {
		if !viper.GetBool(key) {
			failures = append(failures, key)
		}
	}
	if viper.GetFloat64("live.max_order_notional") <= 0 {
		failures = append(failures, "live.max_order_notional")
	}
	if viper.GetFloat64("live.max_daily_loss") <= 0 {
		failures = append(failures, "live.max_daily_loss")
	}
	if strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_KEY")) == "" {
		failures = append(failures, "SYMM_KRAKEN_API_KEY")
	}
	if strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_SECRET")) == "" {
		failures = append(failures, "SYMM_KRAKEN_API_SECRET")
	}
	if viper.GetBool("trading.margin_enabled") {
		failures = append(failures, "trading.margin_enabled")
	}
	failures = append(failures, NativeProtectiveStopsRequired)

	if len(failures) > 0 {
		return fmt.Errorf("live readiness failed: %s", strings.Join(failures, ", "))
	}

	return nil
}

func MaxOrderNotional() float64 {
	return viper.GetFloat64("live.max_order_notional")
}

func MaxDailyLoss() float64 {
	return viper.GetFloat64("live.max_daily_loss")
}

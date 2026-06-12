package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

/*
MarketConfig carries shared market runtime settings loaded once at boot.
*/
type MarketConfig struct {
	AnchorSymbol    string
	QuoteCurrency   string
	BookDepthLevels int
	WSPingInterval  time.Duration
	L3Enabled       bool
	FuturesEnabled  bool
	MarginEnabled   bool
}

func LoadMarketConfig() (MarketConfig, error) {
	pingInterval := viper.GetDuration("market.ws_ping_interval")

	if pingInterval <= 0 {
		return MarketConfig{}, fmt.Errorf("config: market.ws_ping_interval must be positive")
	}

	bookDepth := viper.GetInt("market.book_depth_levels")

	if bookDepth <= 0 {
		return MarketConfig{}, fmt.Errorf("config: market.book_depth_levels must be positive")
	}

	return MarketConfig{
		AnchorSymbol:    viper.GetString("market.anchor_symbol"),
		QuoteCurrency:   viper.GetString("market.quote_currency"),
		BookDepthLevels: bookDepth,
		WSPingInterval:  pingInterval,
		L3Enabled:       viper.GetBool("market.l3_enabled"),
		FuturesEnabled:  viper.GetBool("market.futures_enabled"),
		MarginEnabled:   viper.GetBool("trading.margin_enabled"),
	}, nil
}

/*
PaperWalletBalance returns the configured quote wallet balance for paper trading.
*/
func PaperWalletBalance() (float64, error) {
	quote := strings.ToLower(viper.GetString("market.quote_currency"))
	balance := viper.GetFloat64("trading.paper.wallet." + quote)

	if balance <= 0 {
		return 0, fmt.Errorf(
			"config: trading.paper.wallet.%s must be positive",
			quote,
		)
	}

	return balance, nil
}

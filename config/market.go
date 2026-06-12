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
	DefaultSymbols  []string
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

	config := MarketConfig{
		AnchorSymbol:    strings.ToUpper(strings.TrimSpace(viper.GetString("market.anchor_symbol"))),
		QuoteCurrency:   strings.ToUpper(strings.TrimSpace(viper.GetString("market.quote_currency"))),
		DefaultSymbols:  normalizedSymbols(viper.GetStringSlice("market.default_symbols")),
		BookDepthLevels: bookDepth,
		WSPingInterval:  pingInterval,
		L3Enabled:       viper.GetBool("market.l3_enabled"),
		FuturesEnabled:  viper.GetBool("market.futures_enabled"),
		MarginEnabled:   viper.GetBool("trading.margin_enabled"),
	}

	if err := config.Validate(); err != nil {
		return MarketConfig{}, err
	}

	return config, nil
}

func (config MarketConfig) Validate() error {
	if config.QuoteCurrency == "" {
		return fmt.Errorf("config: market.quote_currency must be non-empty")
	}

	if config.AnchorSymbol == "" {
		return fmt.Errorf("config: market.anchor_symbol must be non-empty")
	}

	if !strings.Contains(config.AnchorSymbol, "/") {
		return fmt.Errorf("config: market.anchor_symbol must be a spot symbol")
	}

	if len(config.DefaultSymbols) == 0 {
		return nil
	}

	for _, symbol := range config.DefaultSymbols {
		if symbol == config.AnchorSymbol {
			return nil
		}
	}

	return fmt.Errorf(
		"config: market.anchor_symbol %q must be listed in market.default_symbols",
		config.AnchorSymbol,
	)
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

func normalizedSymbols(symbols []string) []string {
	normalized := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))

		if symbol == "" {
			continue
		}

		normalized = append(normalized, symbol)
	}

	return normalized
}

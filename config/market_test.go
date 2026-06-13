package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/internal/testconfig"
)

var marketConfigViperKeys = []string{
	"market.quote_currency",
	"market.anchor_symbol",
	"market.default_symbols",
	"market.book_depth_levels",
	"market.ws_ping_interval",
}

func snapshotMarketViperKeys() map[string]any {
	saved := make(map[string]any, len(marketConfigViperKeys))

	for _, key := range marketConfigViperKeys {
		saved[key] = viper.Get(key)
	}

	return saved
}

func restoreMarketViperKeys(saved map[string]any) {
	for key, value := range saved {
		viper.Set(key, value)
	}
}

func TestDefaultPaperConfigDisablesLiveOnlyFeeds(test *testing.T) {
	testconfig.Load(test)

	tradingConfig, tradingErr := LoadTradingConfig()

	if tradingErr != nil {
		test.Fatalf("load trading config: %v", tradingErr)
	}

	if tradingConfig.Model != "paper" {
		test.Fatalf("expected default trading model paper, got %q", tradingConfig.Model)
	}

	marketConfig, marketErr := LoadMarketConfig()

	if marketErr != nil {
		test.Fatalf("load market config: %v", marketErr)
	}

	if marketConfig.FuturesEnabled {
		test.Fatal("default paper config enabled futures websocket")
	}

	if marketConfig.L3Enabled {
		test.Fatal("default paper config enabled L3 websocket")
	}
}

func TestLoadMarketConfigValidatesAnchorSymbol(test *testing.T) {
	saved := snapshotMarketViperKeys()
	defer restoreMarketViperKeys(saved)

	viper.Set("market.quote_currency", "USD")
	viper.Set("market.anchor_symbol", "ETH/USD")
	viper.Set("market.default_symbols", []string{"BTC/USD"})
	viper.Set("market.book_depth_levels", 10)
	viper.Set("market.ws_ping_interval", time.Second)

	_, marketErr := LoadMarketConfig()

	if marketErr == nil {
		test.Fatal("expected anchor/default symbol validation error")
	}
}

func TestLoadMarketConfigRequiresQuoteCurrency(test *testing.T) {
	saved := snapshotMarketViperKeys()
	defer restoreMarketViperKeys(saved)

	viper.Set("market.quote_currency", "")
	viper.Set("market.anchor_symbol", "BTC/USD")
	viper.Set("market.default_symbols", []string{"BTC/USD"})
	viper.Set("market.book_depth_levels", 10)
	viper.Set("market.ws_ping_interval", time.Second)

	_, marketErr := LoadMarketConfig()

	if marketErr == nil {
		test.Fatal("expected quote currency validation error")
	}
}

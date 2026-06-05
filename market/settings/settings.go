package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/quote"
)

const DefaultScanSymbolCap = 64

func ScanSymbolCap() int {
	cap := viper.GetInt("market.max_scan_symbols")

	if cap <= 0 {
		return DefaultScanSymbolCap
	}

	return cap
}

func RequiredBookDepthLevels() (int, error) {
	depth := viper.GetInt("market.book_depth_levels")

	if depth <= 0 {
		return 0, fmt.Errorf("market.book_depth_levels must be positive")
	}

	return depth, nil
}

func RequiredQuoteCurrency() (string, error) {
	currency := quote.NormalizeCurrency(viper.GetString("market.quote_currency"))

	if currency == "" {
		return "", fmt.Errorf("market.quote_currency must be set")
	}

	return currency, nil
}

func RequiredDuration(key string) (time.Duration, error) {
	duration := viper.GetDuration(key)

	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return duration, nil
}

func RequiredFloat(key string) (float64, error) {
	value := viper.GetFloat64(key)

	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return value, nil
}

func RequiredPaperWallet() (float64, error) {
	currency, err := RequiredQuoteCurrency()

	if err != nil {
		return 0, err
	}

	key := "trading.paper.wallet_" + strings.ToLower(currency)
	wallet := viper.GetFloat64(key)

	if wallet <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return wallet, nil
}

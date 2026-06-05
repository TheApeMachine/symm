package market

import (
	"time"

	"github.com/theapemachine/symm/market/quote"
	"github.com/theapemachine/symm/market/settings"
)

const defaultScanSymbolCap = settings.DefaultScanSymbolCap

/*
ScanSymbolCap is the maximum instrument-catalog pairs to subscribe per boot.
When market.max_scan_symbols is zero or negative, the documented default applies.
*/
func ScanSymbolCap() int {
	return settings.ScanSymbolCap()
}

func RequiredBookDepthLevels() (int, error) {
	return settings.RequiredBookDepthLevels()
}

func RequiredQuoteCurrency() (string, error) {
	return settings.RequiredQuoteCurrency()
}

func NormalizeQuoteCurrency(currency string) string {
	return quote.NormalizeCurrency(currency)
}

func SymbolQuote(symbol string) (string, bool) {
	return quote.SymbolQuote(symbol)
}

func SymbolMatchesQuote(symbol string, currency string) bool {
	return quote.SymbolMatchesCurrency(symbol, currency)
}

func RequiredDuration(key string) (time.Duration, error) {
	return settings.RequiredDuration(key)
}

func RequiredFloat(key string) (float64, error) {
	return settings.RequiredFloat(key)
}

func RequiredPaperWallet() (float64, error) {
	return settings.RequiredPaperWallet()
}

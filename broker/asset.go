package broker

import (
	"strings"

	"github.com/spf13/viper"
)

/*
QuoteWalletKey maps configured quote currencies to paper wallet config keys.
*/
func QuoteWalletKey(quote string) string {
	normalized := NormalizeAsset(quote)

	switch normalized {
	case "USD":
		return "usd"
	case "EUR":
		return "eur"
	default:
		return strings.ToLower(normalized)
	}
}

/*
NormalizeAsset strips Kraken-style prefixes and uppercases asset codes.
*/
func NormalizeAsset(asset string) string {
	trimmed := strings.TrimSpace(asset)

	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "Z") && len(trimmed) == 4 {
		return strings.ToUpper(trimmed[1:])
	}

	if strings.HasPrefix(trimmed, "X") && len(trimmed) == 4 {
		return strings.ToUpper(trimmed[1:])
	}

	return strings.ToUpper(trimmed)
}

/*
QuoteAsset returns the configured quote currency in normalized form.
*/
func QuoteAsset() string {
	return NormalizeAsset(viper.GetString("market.quote_currency"))
}

/*
BalanceMatchesQuote reports whether a balance asset row funds the quote wallet.
*/
func BalanceMatchesQuote(assetName string, quote string) bool {
	normalizedAsset := NormalizeAsset(assetName)
	normalizedQuote := NormalizeAsset(quote)

	if normalizedAsset == "" || normalizedQuote == "" {
		return false
	}

	return normalizedAsset == normalizedQuote
}

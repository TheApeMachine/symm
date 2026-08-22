package kraken

import (
	"strings"
)

/*
SpotToFuturesProductID maps a spot pair (e.g. "BTC/USD", "ZEC/USD") to Kraken's
canonical perpetual futures product identifier (e.g. "PF_XBTUSD", "PF_ZECUSD").
*/
func SpotToFuturesProductID(spotSymbol string) string {
	cleaned := strings.TrimSpace(spotSymbol)

	if cleaned == "" {
		return ""
	}

	parts := strings.Split(cleaned, "/")

	if len(parts) != 2 {
		return ""
	}

	base := strings.ToUpper(parts[0])
	quote := strings.ToUpper(parts[1])

	if base == "BTC" {
		base = "XBT"
	}

	if base == "DOGE" {
		base = "XDG"
	}

	return "PF_" + base + quote
}

/*
FuturesProductIDToSpot maps a Kraken perpetual or futures product identifier
(e.g. "PF_XBTUSD", "PF_ZECUSD", "PI_XBTUSD") back to the spot market symbol
(e.g. "BTC/USD", "ZEC/USD").
*/
func FuturesProductIDToSpot(productID string) string {
	cleaned := strings.TrimSpace(productID)

	if cleaned == "" {
		return ""
	}

	trimmed := cleaned

	if strings.HasPrefix(trimmed, "PF_") || strings.HasPrefix(trimmed, "PI_") || strings.HasPrefix(trimmed, "FI_") {
		trimmed = trimmed[3:]
	}

	var base, quote string

	switch {
	case strings.HasSuffix(trimmed, "USD"):
		quote = "USD"
		base = strings.TrimSuffix(trimmed, "USD")
	case strings.HasSuffix(trimmed, "EUR"):
		quote = "EUR"
		base = strings.TrimSuffix(trimmed, "EUR")
	case strings.HasSuffix(trimmed, "GBP"):
		quote = "GBP"
		base = strings.TrimSuffix(trimmed, "GBP")
	case strings.HasSuffix(trimmed, "USDT"):
		quote = "USDT"
		base = strings.TrimSuffix(trimmed, "USDT")
	default:
		return ""
	}

	if base == "XBT" {
		base = "BTC"
	}

	if base == "XDG" {
		base = "DOGE"
	}

	if base == "" || quote == "" {
		return ""
	}

	return base + "/" + quote
}

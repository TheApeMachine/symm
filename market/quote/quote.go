package quote

import "strings"

func NormalizeCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

func SymbolQuote(symbol string) (string, bool) {
	slash := strings.LastIndex(symbol, "/")

	if slash < 0 || slash == len(symbol)-1 {
		return "", false
	}

	symbolQuote := NormalizeCurrency(symbol[slash+1:])

	if symbolQuote == "" {
		return "", false
	}

	return symbolQuote, true
}

func SymbolMatchesCurrency(symbol string, currency string) bool {
	symbolQuote, ok := SymbolQuote(symbol)

	if !ok {
		return false
	}

	return symbolQuote == NormalizeCurrency(currency)
}

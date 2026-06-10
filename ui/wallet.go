package ui

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/user"
)

var ErrMissingQuoteCurrency = errors.New("ui: market.quote_currency is required")

/*
WalletFrame maps Kraken balances into the dashboard wallet header event.
*/
func WalletFrame(balances user.Balances) (map[string]any, error) {
	quote := strings.ToUpper(viper.GetString("market.quote_currency"))

	if quote == "" {
		return nil, ErrMissingQuoteCurrency
	}

	return map[string]any{
		"event":    "wallet",
		"balance":  quoteCash(balances, quote),
		"currency": quote,
	}, nil
}

func quoteCash(balances user.Balances, quote string) float64 {
	for _, asset := range balances.Asset {
		name := strings.ToUpper(asset.Asset)

		if name != quote && name != "Z"+quote {
			continue
		}

		return asset.Balance
	}

	return 0
}

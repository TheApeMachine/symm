package ui

import (
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/user"
)

/*
WalletFrame maps Kraken balances into the dashboard wallet header event.
*/
func WalletFrame(balances user.Balances) map[string]any {
	quote := strings.ToUpper(viper.GetString("market.quote_currency"))

	return map[string]any{
		"event":    "wallet",
		"balance":  quoteCash(balances, quote),
		"currency": quote,
	}
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

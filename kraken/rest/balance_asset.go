package rest

import (
	"fmt"
	"strings"
)

/*
AssetBalance returns one Kraken asset balance by asset code.
*/
func (balance *Balance) AssetBalance(assetCode string) (float64, bool) {
	if balance == nil || len(balance.Result) == 0 || assetCode == "" {
		return 0, false
	}

	candidates := []string{
		strings.ToUpper(assetCode),
		"X" + strings.ToUpper(assetCode),
		"Z" + strings.ToUpper(assetCode),
		"XX" + strings.ToUpper(assetCode),
	}

	seen := make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}

		seen[candidate] = struct{}{}

		raw, ok := balance.Result[candidate]

		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}

		var amount float64

		if _, err := fmt.Sscan(raw, &amount); err != nil {
			continue
		}

		return amount, true
	}

	return 0, false
}

package market

import (
	"github.com/theapemachine/symm/kraken/private"
)

/*
ConfigureLevel3 enables the authenticated L3 feed when Kraken credentials are present.
L3 is market-data only and does not enable live trading.
*/
func ConfigureLevel3(apiKey, apiSecret string) error {
	if apiKey == "" || apiSecret == "" {
		SetLevel3TokenSource(nil)

		return nil
	}

	provider, err := private.NewTokenProvider(apiKey, apiSecret)

	if err != nil {
		return err
	}

	SetLevel3TokenSource(provider)

	return nil
}

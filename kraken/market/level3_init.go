package market

import (
	"github.com/theapemachine/symm/kraken/private"
)

/*
ConfigureLevel3 enables the authenticated L3 feed when Kraken credentials are present.
L3 is market-data only and does not enable live trading. Replay-backed evals use the
recorded L2 book instead; live L3 would dial Kraken on every tune worker.
*/
func ConfigureLevel3(apiKey, apiSecret string) error {
	if replayActive() {
		SetLevel3TokenSource(nil)

		return nil
	}

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

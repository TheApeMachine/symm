package cmd

import (
	"context"
	"fmt"

	kraken "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/private"
)

func configureLevel3(ctx context.Context, apiKey, apiSecret string) error {
	if apiKey == "" || apiSecret == "" {
		kraken.SetOrderTokenSource(nil)

		return nil
	}

	provider, err := private.NewTokenProvider(ctx, apiKey, apiSecret)

	if err != nil {
		return fmt.Errorf("configure level3: %w", err)
	}

	kraken.SetOrderTokenSource(provider)

	return nil
}

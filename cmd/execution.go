package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

/*
WireExecutionAdapter binds exactly one private execution transport for the
configured trading model. Paper and live must never run together.
*/
func WireExecutionAdapter(
	ctx context.Context,
	pool *qpool.Q[any],
	bookStore *krakenmarket.BookStore,
) ([]System, error) {
	model := strings.TrimSpace(viper.GetString("trading.model"))

	switch model {
	case "paper":
		paperRest, restErr := paper.NewRest(ctx)

		if restErr != nil {
			return nil, restErr
		}

		types.BindTokenRest(paperRest)

		return []System{paper.NewWebSocket(ctx, pool, bookStore)}, nil
	case "live":
		apiKey := strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_KEY"))
		apiSecret := strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_SECRET"))

		if apiKey == "" || apiSecret == "" {
			return nil, fmt.Errorf(
				"trading.model=live requires SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET",
			)
		}

		liveRest, restErr := private.NewRest(
			ctx,
			apiKey,
			apiSecret,
			public.EndpointWebSocketsToken,
		)

		if restErr != nil {
			return nil, restErr
		}

		types.BindTokenRest(liveRest)

		return []System{private.NewWebSocket(ctx, pool)}, nil
	default:
		return nil, fmt.Errorf("cmd: unknown trading.model %q", model)
	}
}

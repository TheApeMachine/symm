package market

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
RecentTradeRow is one trade from GET /public/Trades.
Wire: [price, volume, time, side, order type, misc, trade_id].
See https://docs.kraken.com/api/docs/rest-api/get-recent-trades
*/
type RecentTradeRow []any

/*
RecentTrades is trade rows for one internal pair key in the /public/Trades result.
*/
type RecentTrades []RecentTradeRow

/*
RecentTradesResult is the /public/Trades result object (pair keys and "last").
*/
type RecentTradesResult map[string]any

/*
NewRecentTrades fetches recent trades for pair. Pass since 0 to omit since.
*/
func NewRecentTrades(
	ctx context.Context,
	client *public.Rest,
	pair string,
	since int64,
) (RecentTradesResult, error) {
	result := RecentTradesResult{}
	params := fiber.Map{"pair": pair}

	if since > 0 {
		params["since"] = since
	}

	return result, errnie.Error(client.Get(ctx, params, &result))
}

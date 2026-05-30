package market

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
RecentTrade is one executed trade from GET /public/Trades.
Wire row: [price, volume, time, side, order_type, misc, trade_id].
See https://docs.kraken.com/api/docs/rest-api/get-recent-trades
*/
type RecentTrade struct {
	Price     float64   `json:"price"`
	Volume    float64   `json:"volume"`
	Time      time.Time `json:"time"`
	Side      string    `json:"side"`       // "b" buy, "s" sell
	OrderType string    `json:"order_type"` // "m" market, "l" limit
	Misc      string    `json:"misc"`
	TradeID   int64     `json:"trade_id"`
}

/*
RecentTrades is the /public/Trades result: trades keyed by Kraken pair name plus
the pagination cursor. Last is a string nanosecond-since-epoch cursor (distinct
from /public/Spread, whose Last is an integer second).

The executed trade tape: every fill's price, size, aggressor side, order type, and
time, with a cursor to page backward. It is the ground truth of what actually
transacted rather than what was merely quoted -- the aggressor side shows which
side was in control, and the ordered sequence reconstructs realized volume and
price discovery trade by trade.
*/
type RecentTrades struct {
	Last   string                   `json:"last"`
	Trades map[string][]RecentTrade `json:"trades"`
}

/*
NewRecentTrades fetches recent trades for pair. Pass since 0 to omit since.
*/
func NewRecentTrades(
	ctx context.Context,
	client *public.Rest,
	pair string,
	since int64,
) (*RecentTrades, error) {
	trades := &RecentTrades{}
	params := fiber.Map{"pair": pair}

	if since > 0 {
		params["since"] = since
	}

	return trades, errnie.Error(client.Get(ctx, params, trades))
}

package market

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Spread is one best bid/ask sample from GET /public/Spread.
Wire row: [time, bid, ask].
See https://docs.kraken.com/api/docs/rest-api/get-recent-spreads
*/
type Spread struct {
	Time time.Time
	Bid  float64
	Ask  float64
}

/*
SpreadHistory is the /public/Spread result: spread samples keyed by Kraken pair
name plus the pagination cursor (Last is an integer second-since-epoch).

The best bid and ask sampled at every change to the top of book. It is the
exchange's authoritative record of the bid-ask spread through time -- the true,
instantaneous cost of immediacy and a direct measure of how tight or thin
liquidity is, captured without replaying the full book.
*/
type SpreadHistory struct {
	Last    int64
	Spreads map[string][]Spread
}

/*
NewSpread fetches recent spreads for pair. Pass since 0 to omit since.
*/
func NewSpread(
	ctx context.Context,
	client *public.Rest,
	pair string,
	since int64,
) (*SpreadHistory, error) {
	history := &SpreadHistory{}
	params := fiber.Map{"pair": pair}

	if since > 0 {
		params["since"] = since
	}

	return history, errnie.Error(client.Get(ctx, params, history))
}

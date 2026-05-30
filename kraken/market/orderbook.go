package market

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
DepthLevel is one order book level from GET /public/Depth.
Wire row: [price, volume, timestamp].
See https://docs.kraken.com/api/docs/rest-api/get-order-book
*/
type DepthLevel struct {
	Price     float64
	Volume    float64
	Timestamp time.Time
}

/*
DepthBook is the bids and asks for one pair in a Depth response.
*/
type DepthBook struct {
	Asks []DepthLevel `json:"asks"`
	Bids []DepthLevel `json:"bids"`
}

/*
OrderBook is the /public/Depth result keyed by Kraken internal pair name.

A point-in-time snapshot of resting limit liquidity: price, size, and timestamp at
each level on both sides, to a chosen depth. It shows exactly where supply and
demand sit and how much size price must consume to move a given distance -- the
raw material for measuring depth, imbalance, and price impact.
*/
type OrderBook map[string]DepthBook

/*
NewOrderBook fetches the L2 order book. count is the depth per side (pass 0 for default).
*/
func NewOrderBook(
	ctx context.Context,
	client *public.Rest,
	pair string,
	count int,
) (OrderBook, error) {
	book := OrderBook{}
	params := fiber.Map{"pair": pair}

	if count > 0 {
		params["count"] = count
	}

	return book, errnie.Error(client.Get(ctx, params, &book))
}

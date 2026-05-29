package market

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
DepthRow is one level from GET /public/Depth.
Wire: [price, volume, timestamp].
See https://docs.kraken.com/api/docs/rest-api/get-order-book
*/
type DepthRow []any

/*
DepthBook is the bids and asks for one pair in a Depth response.
*/
type DepthBook struct {
	Asks []DepthRow `json:"asks"`
	Bids []DepthRow `json:"bids"`
}

/*
OrderBook is the /public/Depth result keyed by Kraken internal pair name.
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

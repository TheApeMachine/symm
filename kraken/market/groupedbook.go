package market

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
GroupedBookLevel is one grouped price level from GET /public/GroupedBook.
See https://docs.kraken.com/api/docs/rest-api/get-grouped-order-book
*/
type GroupedBookLevel struct {
	Price string `json:"price"`
	Qty   string `json:"qty"`
}

/*
GroupedBook is the /public/GroupedBook result.
*/
type GroupedBook struct {
	Pair     string             `json:"pair"`
	Grouping int                `json:"grouping"`
	Bids     []GroupedBookLevel `json:"bids"`
	Asks     []GroupedBookLevel `json:"asks"`
}

/*
NewGroupedBook fetches the grouped order book for pair at the given tick group size.
*/
func NewGroupedBook(
	ctx context.Context,
	client *public.Rest,
	pair string,
	group int,
) (*GroupedBook, error) {
	book := &GroupedBook{}

	return book, errnie.Error(client.Get(ctx, fiber.Map{
		"pair":  pair,
		"group": group,
	}, book))
}

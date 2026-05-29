package market

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
SpreadRow is one spread sample from GET /public/Spread.
Wire: [time, bid, ask].
See https://docs.kraken.com/api/docs/rest-api/get-recent-spreads
*/
type SpreadRow []any

/*
SpreadHistory is spread rows for one internal pair key in the /public/Spread result.
*/
type SpreadHistory []SpreadRow

/*
SpreadResult is the /public/Spread result object (pair keys and "last").
*/
type SpreadResult map[string]any

/*
NewSpread fetches recent spreads for pair. Pass since 0 to omit since.
*/
func NewSpread(
	ctx context.Context,
	client *public.Rest,
	pair string,
	since int64,
) (SpreadResult, error) {
	result := SpreadResult{}
	params := fiber.Map{"pair": pair}

	if since > 0 {
		params["since"] = since
	}

	return result, errnie.Error(client.Get(ctx, params, &result))
}

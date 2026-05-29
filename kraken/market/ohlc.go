package market

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
OHLCRow is one candle row from GET /public/OHLC.
Wire: [time, open, high, low, close, vwap, volume, count].
See https://docs.kraken.com/api/docs/rest-api/get-ohlc-data
*/
type OHLCRow []any

/*
OHLC is candle rows for one internal pair key in the /public/OHLC result.
*/
type OHLC []OHLCRow

/*
OHLCResult is the /public/OHLC result object (pair keys and "last").
*/
type OHLCResult map[string]any

/*
NewOHLC fetches OHLC data. intervalMinutes is the candle width in minutes.
Pass since 0 to omit the since parameter.
*/
func NewOHLC(
	ctx context.Context,
	client *public.Rest,
	pair string,
	intervalMinutes int,
	since int64,
) (OHLCResult, error) {
	result := OHLCResult{}
	params := fiber.Map{
		"pair":     pair,
		"interval": intervalMinutes,
	}

	if since > 0 {
		params["since"] = since
	}

	return result, errnie.Error(client.Get(ctx, params, &result))
}

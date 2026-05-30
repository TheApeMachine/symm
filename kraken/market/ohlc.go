package market

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Candle is one OHLC candle from GET /public/OHLC.
Wire row: [time, open, high, low, close, vwap, volume, count].
See https://docs.kraken.com/api/docs/rest-api/get-ohlc-data
*/
type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	VWAP   float64
	Volume float64
	Count  int64
}

/*
OHLC is the /public/OHLC result: candles keyed by Kraken pair name plus the
pagination cursor (Last is an integer second-since-epoch).

Candlestick bars compress every trade in a fixed interval into open, high, low,
and close, plus the interval's volume-weighted average price and trade count. It
is the standard summary of price action and participation over time -- VWAP gives
the fair transacted price -- served historically and paginated far beyond what
live feeds retain.
*/
type OHLC struct {
	Last    int64
	Candles map[string][]Candle
}

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
) (*OHLC, error) {
	ohlc := &OHLC{}
	params := fiber.Map{
		"pair":     pair,
		"interval": intervalMinutes,
	}

	if since > 0 {
		params["since"] = since
	}

	return ohlc, errnie.Error(client.Get(ctx, params, ohlc))
}

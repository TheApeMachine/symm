package market

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
TickerQuote is Kraken's [price, wholeLotVolume, lotVolume] tuple for the best
ask ("a") and best bid ("b") in a /public/Ticker entry.
*/
type TickerQuote struct {
	Price          float64
	WholeLotVolume float64
	LotVolume      float64
}

/*
TickerClose is the last-trade-closed [price, lotVolume] tuple ("c").
*/
type TickerClose struct {
	Price     float64
	LotVolume float64
}

/*
DayValue is Kraken's [today, last24Hours] tuple used for volume ("v"), VWAP
("p"), low ("l"), and high ("h").
*/
type DayValue struct {
	Today       float64
	Last24Hours float64
}

/*
DayCount is the number-of-trades [today, last24Hours] tuple ("t").
*/
type DayCount struct {
	Today       int64
	Last24Hours int64
}

/*
TickerEntry is one pair's ticker from GET /public/Ticker.
See https://docs.kraken.com/api/docs/rest-api/get-ticker-information
*/
type TickerEntry struct {
	Ask       TickerQuote `json:"a"`
	Bid       TickerQuote `json:"b"`
	LastTrade TickerClose `json:"c"`
	Volume    DayValue    `json:"v"`
	VWAP      DayValue    `json:"p"`
	Trades    DayCount    `json:"t"`
	Low       DayValue    `json:"l"`
	High      DayValue    `json:"h"`
	Open      float64     `json:"o"`
}

/*
TickerInfo is the /public/Ticker result keyed by Kraken internal pair name.

A rolling 24-hour summary per market: best bid and ask with sizes, the last trade,
today's and 24-hour volume, VWAP, high, low, trade count, and the opening price.
It is a single compact snapshot of where a market stands and how active it has
been -- enough to rank, compare, and screen the whole universe in one request.
*/
type TickerInfo map[string]TickerEntry

/*
NewTickerInfo fetches ticker information. Pass no pairNames for all markets.
*/
func NewTickerInfo(
	ctx context.Context,
	client *public.Rest,
	pairNames ...string,
) (TickerInfo, error) {
	info := TickerInfo{}
	params := fiber.Map{}

	if len(pairNames) > 0 {
		params["pair"] = strings.Join(pairNames, ",")
	}

	return info, errnie.Error(client.Get(ctx, params, &info))
}

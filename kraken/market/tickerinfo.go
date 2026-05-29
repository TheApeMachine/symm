package market

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
TickerEntry is one pair row from GET /public/Ticker.
See https://docs.kraken.com/api/docs/rest-api/get-ticker-information
*/
type TickerEntry struct {
	Ask       []string  `json:"a"`
	Bid       []string  `json:"b"`
	LastTrade []string  `json:"c"`
	Volume    []string  `json:"v"`
	VWAP      []string  `json:"p"`
	Trades    []float64 `json:"t"`
	Low       []string  `json:"l"`
	High      []string  `json:"h"`
	Open      string    `json:"o"`
}

/*
TickerInfo is the /public/Ticker result keyed by Kraken internal pair name.
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

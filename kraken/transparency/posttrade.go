package transparency

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Trade is one executed spot trade from Kraken transparency post-trade data.
See https://docs.kraken.com/api/docs/rest-api/get-post-trade
*/
type Trade struct {
	TradeID           string    `json:"trade_id"`
	Price             string    `json:"price"`
	Quantity          string    `json:"quantity"`
	Symbol            string    `json:"symbol"`
	Description       string    `json:"description"`
	BaseAsset         string    `json:"base_asset"`
	BaseNotation      string    `json:"base_notation"`
	BaseDtiCode       string    `json:"base_dti_code"`
	BaseDtiShortName  string    `json:"base_dti_short_name"`
	QuoteAsset        string    `json:"quote_asset"`
	QuoteNotation     string    `json:"quote_notation"`
	QuoteDtiCode      string    `json:"quote_dti_code"`
	QuoteDtiShortName string    `json:"quote_dti_short_name"`
	TradeVenue        string    `json:"trade_venue"`
	TradeTs           time.Time `json:"trade_ts"`
	PublicationVenue  string    `json:"publication_venue"`
	PublicationTs     time.Time `json:"publication_ts"`
}

/*
PostTrade is the post-trade transparency payload for one symbol.
*/
type PostTrade struct {
	LastTs time.Time `json:"last_ts"`
	Count  int       `json:"count"`
	Trades []Trade   `json:"trades"`
}

/*
NewPostTrade fetches post-trade transparency data for symbol.
Pass zero fromTs or toTs to omit that bound; pass count 0 for the exchange default.
*/
func NewPostTrade(
	ctx context.Context,
	client *public.Rest,
	symbol string,
	fromTs time.Time,
	toTs time.Time,
	count int,
) (*PostTrade, error) {
	posttrade := &PostTrade{}
	params := fiber.Map{
		"symbol": symbol,
	}

	if !fromTs.IsZero() {
		params["from_ts"] = fromTs.UTC().Format(time.RFC3339Nano)
	}

	if !toTs.IsZero() {
		params["to_ts"] = toTs.UTC().Format(time.RFC3339Nano)
	}

	if count > 0 {
		params["count"] = count
	}

	return posttrade, errnie.Error(client.Get(ctx, params, posttrade))
}

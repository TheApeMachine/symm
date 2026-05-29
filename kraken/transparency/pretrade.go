package transparency

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

type Bid struct {
	Side          string    `json:"side"`
	Price         string    `json:"price"`
	Qty           string    `json:"qty"`
	Count         int       `json:"count"`
	PublicationTs time.Time `json:"publication_ts"`
	SubmissionTs  time.Time `json:"submission_ts"`
}

type Ask struct {
	Side          string    `json:"side"`
	Price         string    `json:"price"`
	Qty           string    `json:"qty"`
	Count         int       `json:"count"`
	PublicationTs time.Time `json:"publication_ts"`
	SubmissionTs  time.Time `json:"submission_ts"`
}

type PreTrade struct {
	Symbol            string `json:"symbol"`
	Description       string `json:"description"`
	BaseAsset         string `json:"base_asset"`
	BaseNotation      string `json:"base_notation"`
	BaseDtiCode       string `json:"base_dti_code"`
	BaseDtiShortName  string `json:"base_dti_short_name"`
	QuoteAsset        string `json:"quote_asset"`
	QuoteNotation     string `json:"quote_notation"`
	QuoteDtiCode      string `json:"quote_dti_code"`
	QuoteDtiShortName string `json:"quote_dti_short_name"`
	Venue             string `json:"venue"`
	System            string `json:"system"`
}

func NewPreTrade(
	ctx context.Context, client *public.Rest, symbolPairs []string,
) (*PreTrade, error) {
	pretrade := &PreTrade{}

	return pretrade, errnie.Error(client.Get(ctx, fiber.Map{
		"method": "pretrade",
		"params": fiber.Map{
			"symbols": symbolPairs,
		},
	}, pretrade))
}

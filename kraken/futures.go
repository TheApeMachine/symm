package kraken

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
FuturesTickerData holds one ticker snapshot from the Kraken Futures WebSocket
ticker feed. It provides real-time pricing, open interest, mark price, index
price, and funding rate predictions.
*/
type FuturesTickerData struct {
	ProductID             string           `json:"product_id"`
	Symbol                string           `json:"symbol"`
	Bid                   *decimal.Decimal `json:"bid"`
	BidSize               float64          `json:"bid_size"`
	Ask                   *decimal.Decimal `json:"ask"`
	AskSize               float64          `json:"ask_size"`
	Last                  *decimal.Decimal `json:"last"`
	OpenInterest          float64          `json:"openInterest"`
	MarkPrice             *decimal.Decimal `json:"markPrice"`
	IndexPrice            *decimal.Decimal `json:"indexPrice"`
	FundingRate           *decimal.Decimal `json:"funding_rate"`
	FundingRatePrediction *decimal.Decimal `json:"funding_rate_prediction"`
	Volume                float64          `json:"volume"`
	Timestamp             time.Time        `json:"timestamp"`
	SyntheticTimestamp    bool             `json:"synthetic_timestamp,omitempty"`
}

/*
FuturesTicker wraps incoming ticker feed messages from Kraken Futures.
*/
type FuturesTicker struct {
	Feed string            `json:"feed"`
	Data FuturesTickerData `json:"-"`
}

/*
FuturesTradeData is one individual execution or liquidation from the Kraken
Futures trade feed.
*/
type FuturesTradeData struct {
	ProductID          string          `json:"product_id"`
	Symbol             string          `json:"symbol"`
	Price              decimal.Decimal `json:"price"`
	Qty                float64         `json:"qty"`
	Side               string          `json:"side"`
	Type               string          `json:"type"`
	UID                string          `json:"uid"`
	Timestamp          time.Time       `json:"timestamp"`
	SyntheticTimestamp bool            `json:"synthetic_timestamp,omitempty"`
}

/*
FuturesTrade wraps trade feed messages from Kraken Futures.
*/
type FuturesTrade struct {
	Feed string             `json:"feed"`
	Data []FuturesTradeData `json:"data"`
}

/*
FuturesBookData holds order book snapshots and deltas from Kraken Futures.
*/
type FuturesBookData struct {
	ProductID          string      `json:"product_id"`
	Symbol             string      `json:"symbol"`
	Bids               []BookLevel `json:"bids"`
	Asks               []BookLevel `json:"asks"`
	Timestamp          time.Time   `json:"timestamp"`
	SyntheticTimestamp bool        `json:"synthetic_timestamp,omitempty"`
}

/*
FuturesBook wraps order book feed messages from Kraken Futures.
*/
type FuturesBook struct {
	Feed string          `json:"feed"`
	Data FuturesBookData `json:"data"`
}

/*
FuturesSubscription constructs a typed subscription message for Kraken Futures.
*/
type FuturesSubscription struct {
	Event      string   `json:"event"`
	Feed       string   `json:"feed"`
	ProductIDs []string `json:"product_ids,omitempty"`
}

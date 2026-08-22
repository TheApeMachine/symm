package kraken

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
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
}

/*
FuturesTicker wraps incoming ticker feed messages from Kraken Futures.
*/
type FuturesTicker struct {
	Feed string            `json:"feed"`
	Data FuturesTickerData `json:"-"`
}

/*
NewFuturesTicker parses raw wire bytes from the Kraken Futures ticker feed.
*/
func NewFuturesTicker(buffer []byte) *FuturesTicker {
	futuresTicker := &FuturesTicker{}

	var rawMap map[string]any

	if err := sonic.Unmarshal(buffer, &rawMap); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"futures: invalid ticker payload",
			err,
		))

		return futuresTicker
	}

	if feedName, ok := rawMap["feed"].(string); ok {
		futuresTicker.Feed = feedName
	}

	futuresTicker.Data = parseFuturesTickerData(rawMap)
	return futuresTicker
}

func parseFuturesTickerData(rawMap map[string]any) FuturesTickerData {
	tickerData := FuturesTickerData{}

	if productID, ok := rawMap["product_id"].(string); ok {
		tickerData.ProductID = productID
	}

	if symbol, ok := rawMap["symbol"].(string); ok {
		tickerData.Symbol = symbol
	}

	tickerData.Bid = extractDecimal(rawMap, "bid")
	tickerData.BidSize = extractFloat(rawMap, "bid_size")
	tickerData.Ask = extractDecimal(rawMap, "ask")
	tickerData.AskSize = extractFloat(rawMap, "ask_size")
	tickerData.Last = extractDecimal(rawMap, "last")
	tickerData.OpenInterest = extractFloatAlt(rawMap, "openInterest", "open_interest")
	tickerData.MarkPrice = extractDecimalAlt(rawMap, "markPrice", "mark_price")
	tickerData.IndexPrice = extractDecimalAlt(rawMap, "indexPrice", "index_price")
	tickerData.FundingRate = extractDecimalAlt(rawMap, "funding_rate", "fundingRate")
	tickerData.FundingRatePrediction = extractDecimalAlt(rawMap, "funding_rate_prediction", "fundingRatePrediction")
	tickerData.Volume = extractFloatAlt(rawMap, "volume", "vol24h")
	tickerData.Timestamp = extractTimestamp(rawMap)

	return tickerData
}

/*
FuturesTradeData is one individual execution or liquidation from the Kraken
Futures trade feed.
*/
type FuturesTradeData struct {
	ProductID string          `json:"product_id"`
	Symbol    string          `json:"symbol"`
	Price     decimal.Decimal `json:"price"`
	Qty       float64         `json:"qty"`
	Side      string          `json:"side"`
	Type      string          `json:"type"`
	UID       string          `json:"uid"`
	Timestamp time.Time       `json:"timestamp"`
}

/*
FuturesTrade wraps trade feed messages from Kraken Futures.
*/
type FuturesTrade struct {
	Feed string             `json:"feed"`
	Data []FuturesTradeData `json:"data"`
}

/*
NewFuturesTrade parses raw wire bytes from the Kraken Futures trade feed.
*/
func NewFuturesTrade(buffer []byte) *FuturesTrade {
	futuresTrade := &FuturesTrade{Data: []FuturesTradeData{}}

	var rawMap map[string]any

	if err := sonic.Unmarshal(buffer, &rawMap); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"futures: invalid trade payload",
			err,
		))

		return futuresTrade
	}

	feedName, _ := rawMap["feed"].(string)
	futuresTrade.Feed = feedName

	if feedName == "trade_snapshot" {
		if tradesSlice, ok := rawMap["trades"].([]any); ok {
			for _, item := range tradesSlice {
				if itemMap, ok := item.(map[string]any); ok {
					futuresTrade.Data = append(futuresTrade.Data, parseSingleFuturesTrade(itemMap))
				}
			}
		}

		return futuresTrade
	}

	futuresTrade.Data = append(futuresTrade.Data, parseSingleFuturesTrade(rawMap))
	return futuresTrade
}

func parseSingleFuturesTrade(rawMap map[string]any) FuturesTradeData {
	tradeData := FuturesTradeData{}

	if productID, ok := rawMap["product_id"].(string); ok {
		tradeData.ProductID = productID
	}

	if symbol, ok := rawMap["symbol"].(string); ok {
		tradeData.Symbol = symbol
	}

	if side, ok := rawMap["side"].(string); ok {
		tradeData.Side = side
	}

	if tradeType, ok := rawMap["type"].(string); ok {
		tradeData.Type = tradeType
	}

	if fillType, ok := rawMap["fill_type"].(string); ok && tradeData.Type == "" {
		tradeData.Type = fillType
	}

	if uid, ok := rawMap["uid"].(string); ok {
		tradeData.UID = uid
	}

	if priceDec := extractDecimal(rawMap, "price"); priceDec != nil {
		tradeData.Price = *priceDec
	}

	tradeData.Qty = extractFloatAlt(rawMap, "qty", "size")
	tradeData.Timestamp = extractTimestamp(rawMap)

	return tradeData
}

/*
FuturesBookData holds order book snapshots and deltas from Kraken Futures.
*/
type FuturesBookData struct {
	ProductID string      `json:"product_id"`
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Timestamp time.Time   `json:"timestamp"`
}

/*
FuturesBook wraps order book feed messages from Kraken Futures.
*/
type FuturesBook struct {
	Feed string          `json:"feed"`
	Data FuturesBookData `json:"data"`
}

/*
NewFuturesBook parses raw wire bytes from the Kraken Futures book feed.
*/
func NewFuturesBook(buffer []byte) *FuturesBook {
	futuresBook := &FuturesBook{}

	var rawMap map[string]any

	if err := sonic.Unmarshal(buffer, &rawMap); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"futures: invalid book payload",
			err,
		))

		return futuresBook
	}

	if feedName, ok := rawMap["feed"].(string); ok {
		futuresBook.Feed = feedName
	}

	bookData := FuturesBookData{
		Bids: make([]BookLevel, 0),
		Asks: make([]BookLevel, 0),
	}

	if productID, ok := rawMap["product_id"].(string); ok {
		bookData.ProductID = productID
	}

	bookData.Timestamp = extractTimestamp(rawMap)

	if bidsSlice, ok := rawMap["bids"].([]any); ok {
		bookData.Bids = parseBookLevels(bidsSlice)
	}

	if asksSlice, ok := rawMap["asks"].([]any); ok {
		bookData.Asks = parseBookLevels(asksSlice)
	}

	futuresBook.Data = bookData
	return futuresBook
}

func parseBookLevels(levelsSlice []any) []BookLevel {
	levels := make([]BookLevel, 0, len(levelsSlice))

	for _, item := range levelsSlice {
		levelMap, ok := item.(map[string]any)

		if !ok {
			continue
		}

		priceDec := extractDecimal(levelMap, "price")

		if priceDec == nil {
			continue
		}

		qty := extractFloatAlt(levelMap, "qty", "size")

		levels = append(levels, BookLevel{
			Price: *priceDec,
			Qty:   qty,
		})
	}

	return levels
}

/*
FuturesSubscription constructs a typed subscription message for Kraken Futures.
*/
type FuturesSubscription struct {
	Event      string   `json:"event"`
	Feed       string   `json:"feed"`
	ProductIDs []string `json:"product_ids,omitempty"`
}

/*
NewFuturesSubscription creates a new subscription payload.
*/
func NewFuturesSubscription(feed string, productIDs []string) FuturesSubscription {
	return FuturesSubscription{
		Event:      "subscribe",
		Feed:       feed,
		ProductIDs: productIDs,
	}
}

func (subscription FuturesSubscription) MarshalJSON() ([]byte, error) {
	type alias FuturesSubscription
	return sonic.Marshal(alias(subscription))
}

func extractDecimal(rawMap map[string]any, key string) *decimal.Decimal {
	value, exists := rawMap[key]

	if !exists || value == nil {
		return nil
	}

	switch v := value.(type) {
	case float64:
		return decimal.NewFromFloat64(v)
	case string:
		dec, err := decimal.NewFromString(v)

		if err != nil {
			return nil
		}

		return dec
	case json.Number:
		dec, err := decimal.NewFromString(v.String())

		if err != nil {
			return nil
		}

		return dec
	default:
		return nil
	}
}

func extractDecimalAlt(rawMap map[string]any, keyOne, keyTwo string) *decimal.Decimal {
	if dec := extractDecimal(rawMap, keyOne); dec != nil {
		return dec
	}

	return extractDecimal(rawMap, keyTwo)
}

func extractFloat(rawMap map[string]any, key string) float64 {
	value, exists := rawMap[key]

	if !exists || value == nil {
		return 0
	}

	switch v := value.(type) {
	case float64:
		return v
	case string:
		dec, err := decimal.NewFromString(v)

		if err != nil {
			return 0
		}

		return dec.Float64()
	case json.Number:
		floatVal, _ := v.Float64()
		return floatVal
	default:
		return 0
	}
}

func extractFloatAlt(rawMap map[string]any, keyOne, keyTwo string) float64 {
	if val := extractFloat(rawMap, keyOne); val != 0 {
		return val
	}

	return extractFloat(rawMap, keyTwo)
}

func extractTimestamp(rawMap map[string]any) time.Time {
	if timeVal, exists := rawMap["time"]; exists && timeVal != nil {
		if epochMs, ok := timeVal.(float64); ok && epochMs > 0 {
			return time.UnixMilli(int64(epochMs)).UTC()
		}
	}

	if timestampVal, exists := rawMap["timestamp"]; exists && timestampVal != nil {
		if epochMs, ok := timestampVal.(float64); ok && epochMs > 0 {
			return time.UnixMilli(int64(epochMs)).UTC()
		}
	}

	return time.Now().UTC()
}

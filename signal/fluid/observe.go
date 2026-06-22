package fluid

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/theapemachine/datura"
)

func (signal *Signal) hydrateRegistryFromTree() {
	if signal == nil || signal.registry == nil || signal.tree == nil {
		return
	}

	for _, role := range []string{"book", "trade", "ticker"} {
		prefix := role + "/"

		for inbound := range signal.tree.Seek([]byte(prefix)) {
			switch role {
			case "book":
				signal.observeBookArtifact(inbound)
			case "trade":
				signal.observeTradeArtifact(inbound)
			case "ticker":
				signal.observeTickerArtifact(inbound)
			}
		}
	}
}

func artifactEventAt(artifact *datura.Artifact, fallback time.Time) time.Time {
	if artifact != nil {
		if timestamp := artifact.Timestamp(); timestamp > 0 {
			return time.Unix(0, timestamp).UTC()
		}
	}

	if !fallback.IsZero() {
		return fallback
	}

	return time.Now()
}

func (signal *Signal) observeBookArtifact(artifact *datura.Artifact) {
	var update BookUpdate

	if json.Unmarshal(artifact.DecryptPayload(), &update) == nil && update.Symbol != "" {
		if update.Timestamp.IsZero() {
			update.Timestamp = artifactEventAt(artifact, time.Time{})
		}

		signal.observeBookUpdate(update)

		return
	}

	var updates []BookUpdate

	if json.Unmarshal(artifact.DecryptPayload(), &updates) == nil {
		for _, row := range updates {
			if row.Timestamp.IsZero() {
				row.Timestamp = artifactEventAt(artifact, time.Time{})
			}

			signal.observeBookUpdate(row)
		}

		return
	}

	scope, _ := artifact.Scope()

	if scope == "" {
		return
	}

	eventAt := artifactEventAt(artifact, time.Time{})

	if timestamp, timestampOK := peekElementOK[time.Time](artifact.DecryptPayload(), "timestamp"); timestampOK {
		eventAt = timestamp
	}

	decoded := bookElementToKraken(scope, artifact.DecryptPayload(), eventAt)

	if decoded.Symbol == "" {
		return
	}

	if len(decoded.Bids) == 0 && len(decoded.Asks) == 0 {
		return
	}

	signal.observeBookUpdate(decoded)
}

func (signal *Signal) observeBookUpdate(update BookUpdate) {
	if update.Symbol == "" {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	state := signal.registry.loadSymbol(update.Symbol)

	if state == nil {
		return
	}

	_ = state.FeedBook(update, eventAt)
}

func artifactPayload(artifact *datura.Artifact) ([]byte, bool) {
	if artifact == nil || !artifact.HasEncryptedPayload() {
		return nil, false
	}

	payload := artifact.DecryptPayload()

	if len(payload) == 0 {
		return nil, false
	}

	return payload, true
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifactPayload(artifact)

	if !payloadOK {
		return
	}

	var update TradeUpdate

	if json.Unmarshal(payload, &update) == nil && update.Symbol != "" {
		if update.Timestamp.IsZero() {
			update.Timestamp = artifactEventAt(artifact, time.Time{})
		}

		signal.observeTradeUpdate(update)

		return
	}

	var updates []TradeUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, row := range updates {
		if row.Timestamp.IsZero() {
			row.Timestamp = artifactEventAt(artifact, time.Time{})
		}

		signal.observeTradeUpdate(row)
	}
}

func (signal *Signal) observeTradeUpdate(update TradeUpdate) {
	if update.Symbol == "" || update.Price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	state := signal.registry.loadSymbol(update.Symbol)

	if state == nil {
		return
	}

	_ = state.FeedTrade(eventAt, update.Price, update.Qty, update.Side)
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifactPayload(artifact)

	if !payloadOK {
		return
	}

	var update TickerUpdate

	if json.Unmarshal(payload, &update) == nil && update.Symbol != "" {
		if update.Timestamp.IsZero() {
			update.Timestamp = artifactEventAt(artifact, time.Time{})
		}

		signal.observeTickerUpdate(update)

		return
	}

	var updates []TickerUpdate

	if json.Unmarshal(payload, &updates) != nil {
		scope, _ := artifact.Scope()

		if scope == "" {
			return
		}

		eventAt := time.Now()

		if timestamp, timestampOK := peekElementOK[time.Time](payload, "timestamp"); timestampOK {
			eventAt = timestamp
		}

		decoded := tickerElementToKraken(scope, payload, eventAt)
		signal.observeTickerUpdate(decoded)

		return
	}

	for _, row := range updates {
		if row.Timestamp.IsZero() {
			row.Timestamp = artifactEventAt(artifact, time.Time{})
		}

		signal.observeTickerUpdate(row)
	}
}

func (signal *Signal) observeTickerUpdate(update TickerUpdate) {
	if update.Symbol == "" {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	state := signal.registry.loadSymbol(update.Symbol)

	if state == nil {
		return
	}

	_ = state.FeedTicker(update, eventAt)
}

func peekElementOK[T any](element []byte, path string) (T, bool) {
	var zero T

	if len(element) == 0 {
		return zero, false
	}

	artifact := datura.Acquire("element", datura.Artifact_Type_json)

	if artifact.WithPayload(element) == nil {
		artifact.Release()

		return zero, false
	}

	value := datura.Peek[T](artifact, path)
	artifact.Release()

	return value, true
}

func elementTime(element []byte, key string) (time.Time, bool) {
	return peekElementOK[time.Time](element, key)
}

func eachBookLevelElement(
	element []byte,
	key string,
	visit func(price float64, qty float64),
) {
	for index := 0; ; index++ {
		price, priceOK := peekElementOK[float64](element, fmt.Sprintf("%s.%d.price", key, index))
		qty, qtyOK := peekElementOK[float64](element, fmt.Sprintf("%s.%d.qty", key, index))

		if !priceOK || !qtyOK {
			break
		}

		visit(price, qty)
	}
}

func bookElementToKraken(symbol string, element []byte, eventAt time.Time) BookUpdate {
	update := BookUpdate{
		Symbol:    symbol,
		Timestamp: eventAt,
	}

	if feedType, feedTypeOK := peekElementOK[string](element, "feed_type"); feedTypeOK && feedType != "" {
		update.Type = feedType
	}

	if bookType, bookTypeOK := peekElementOK[string](element, "type"); bookTypeOK && bookType != "" && update.Type == "" {
		update.Type = bookType
	}

	if timestamp, timestampOK := elementTime(element, "timestamp"); timestampOK {
		update.Timestamp = timestamp
	}

	eachBookLevelElement(element, "bids", func(price float64, qty float64) {
		update.Bids = append(update.Bids, BookLevel{Price: price, Qty: qty})
	})

	eachBookLevelElement(element, "asks", func(price float64, qty float64) {
		update.Asks = append(update.Asks, BookLevel{Price: price, Qty: qty})
	})

	if update.Symbol == "" {
		update.Symbol = symbol
	}

	if update.Timestamp.IsZero() {
		update.Timestamp = eventAt
	}

	if update.Type == "" {
		update.Type = "update"
	}

	return update
}

func tickerElementToKraken(symbol string, element []byte, eventAt time.Time) TickerUpdate {
	update := TickerUpdate{Symbol: symbol, Timestamp: eventAt}

	if ask, ok := peekElementOK[float64](element, "ask"); ok {
		update.Ask = ask
	}

	if askQty, ok := peekElementOK[float64](element, "ask_qty"); ok {
		update.AskQty = askQty
	}

	if bid, ok := peekElementOK[float64](element, "bid"); ok {
		update.Bid = bid
	}

	if bidQty, ok := peekElementOK[float64](element, "bid_qty"); ok {
		update.BidQty = bidQty
	}

	if change, ok := peekElementOK[float64](element, "change"); ok {
		update.Change = change
	}

	if changePct, ok := peekElementOK[float64](element, "change_pct"); ok {
		update.ChangePct = changePct
	}

	if high, ok := peekElementOK[float64](element, "high"); ok {
		update.High = high
	}

	if last, ok := peekElementOK[float64](element, "last"); ok {
		update.Last = last
	}

	if low, ok := peekElementOK[float64](element, "low"); ok {
		update.Low = low
	}

	if volume, ok := peekElementOK[float64](element, "volume"); ok {
		update.Volume = volume
	}

	if vwap, ok := peekElementOK[float64](element, "vwap"); ok {
		update.VWAP = vwap
	}

	if timestamp, ok := elementTime(element, "timestamp"); ok {
		update.Timestamp = timestamp
	}

	update.Symbol = symbol

	if update.Timestamp.IsZero() {
		update.Timestamp = eventAt
	}

	return update
}

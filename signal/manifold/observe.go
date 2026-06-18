package manifold

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
)

func (signal *Signal) hydrateFieldFromTree() {
	if signal == nil || signal.field == nil || signal.tree == nil {
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

func (signal *Signal) observeBookArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var update BookUpdate

	if json.Unmarshal(payload, &update) == nil && update.Symbol != "" {
		signal.observeBookUpdate(update)
		return
	}

	var updates []BookUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, row := range updates {
		signal.observeBookUpdate(row)
	}
}

func (signal *Signal) observeBookUpdate(update BookUpdate) {
	if update.Symbol == "" {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	_ = signal.field.enqueueBook(update, eventAt)
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var update TradeUpdate

	if json.Unmarshal(payload, &update) == nil && update.Symbol != "" {
		signal.observeTradeUpdate(update)
		return
	}

	var updates []TradeUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, row := range updates {
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

	row := update
	_ = signal.field.enqueueTrade(&row, eventAt)
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var update TickerUpdate

	if json.Unmarshal(payload, &update) == nil && update.Symbol != "" {
		signal.observeTickerUpdate(update)
		return
	}

	var updates []TickerUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, row := range updates {
		signal.observeTickerUpdate(row)
	}
}

func (signal *Signal) observeTickerUpdate(update TickerUpdate) {
	if update.Symbol == "" {
		return
	}

	price := update.Last

	if price <= 0 && update.Bid > 0 && update.Ask > update.Bid {
		price = (update.Bid + update.Ask) / 2
	}

	if price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	row := update
	row.Last = price
	_ = signal.field.enqueueTicker(row, eventAt)
}

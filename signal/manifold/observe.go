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

	var latest int64

	for _, role := range []string{"book", "trade", "ticker"} {
		prefix := role + "/"

		for inbound := range signal.tree.Seek([]byte(prefix)) {
			stamp := inbound.Timestamp()

			if stamp <= signal.lastHydrateStamp {
				continue
			}

			switch role {
			case "book":
				signal.observeBookArtifact(inbound)
			case "trade":
				signal.observeTradeArtifact(inbound)
			case "ticker":
				signal.observeTickerArtifact(inbound)
			}

			if stamp > latest {
				latest = stamp
			}
		}
	}

	if latest > signal.lastHydrateStamp {
		signal.lastHydrateStamp = latest
	}
}

func treeArtifactPayload(artifact *datura.Artifact) []byte {
	if artifact == nil || !artifact.HasEncryptedPayload() {
		return nil
	}

	return artifact.DecryptPayload()
}

func forEachKrakenElement(payload []byte, visit func(element []byte)) {
	if len(payload) == 0 || visit == nil {
		return
	}

	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}

	if json.Unmarshal(payload, &envelope) == nil && len(envelope.Data) > 0 {
		for _, row := range envelope.Data {
			if len(row) > 0 {
				visit(row)
			}
		}

		return
	}

	visit(payload)
}

func (signal *Signal) observeBookArtifact(artifact *datura.Artifact) {
	payload := treeArtifactPayload(artifact)

	if len(payload) == 0 {
		return
	}

	forEachKrakenElement(payload, func(element []byte) {
		var update BookUpdate

		if json.Unmarshal(element, &update) == nil && update.Symbol != "" {
			signal.observeBookUpdate(update)
		}
	})
}

func (signal *Signal) observeBookUpdate(update BookUpdate) {
	if update.Symbol == "" {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	_ = signal.field.enqueueBook(update, eventAt)
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	payload := treeArtifactPayload(artifact)

	if len(payload) == 0 {
		return
	}

	forEachKrakenElement(payload, func(element []byte) {
		var update TradeUpdate

		if json.Unmarshal(element, &update) == nil && update.Symbol != "" {
			signal.observeTradeUpdate(update)
		}
	})
}

func (signal *Signal) observeTradeUpdate(update TradeUpdate) {
	if update.Symbol == "" || update.Price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	row := update
	_ = signal.field.enqueueTrade(&row, eventAt)
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	payload := treeArtifactPayload(artifact)

	if len(payload) == 0 {
		return
	}

	forEachKrakenElement(payload, func(element []byte) {
		var update TickerUpdate

		if json.Unmarshal(element, &update) == nil && update.Symbol != "" {
			signal.observeTickerUpdate(update)
		}
	})
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
		eventAt = time.Now().UTC()
	}

	row := update
	row.Last = price
	_ = signal.field.enqueueTicker(row, eventAt)
}

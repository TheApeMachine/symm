package leadlag

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
)

type tradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Timestamp time.Time `json:"timestamp"`
}

type tickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Timestamp time.Time `json:"timestamp"`
}

func (signal *Signal) hydrateSectionFromTree() {
	section, _ := NewSectionFromConfig()
	signal.Section = section

	for _, role := range []string{"trade", "ticker"} {
		prefix := role + "/"

		for inbound := range signal.tree.Seek([]byte(prefix)) {
			switch role {
			case "trade":
				signal.observeTradeArtifact(inbound)
			case "ticker":
				signal.observeTickerArtifact(inbound)
			}
		}
	}

	signal.warmAnchorBaseline()
}

func (signal *Signal) warmAnchorBaseline() {
	anchor := signal.Section.anchorState()

	if anchor == nil {
		return
	}

	for range len(anchor.prices) {
		signal.Section.anchorMove()
	}
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var updates []tradeUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, update := range updates {
		signal.observeTradeUpdate(update)
	}
}

func (signal *Signal) observeTradeUpdate(update tradeUpdate) {
	if update.Symbol == "" || update.Price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	signal.Section.ObservePrice(update.Symbol, update.Price, eventAt)
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return
	}

	var updates []tickerUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, update := range updates {
		signal.observeTickerUpdate(update)
	}
}

func (signal *Signal) observeTickerUpdate(update tickerUpdate) {
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

	signal.Section.ObservePrice(update.Symbol, price, eventAt)
}

package correlation

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
	marketsection "github.com/theapemachine/symm/market"
)

type tradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

type tickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	BidQty    float64   `json:"bid_qty"`
	AskQty    float64   `json:"ask_qty"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

func (signal *Signal) hydrateCrossSectionFromTree() {
	crossSection, crossSectionErr := marketsection.NewCrossSection(&signal.crossSectionCfg)

	if crossSectionErr != nil {
		return
	}

	signal.CrossSection = crossSection

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

	qty := update.Qty

	if qty <= 0 {
		qty = 1
	}

	row, rowErr := marketsection.NewSymbolRow(
		update.Symbol,
		update.Price,
		0,
		update.Price*qty,
		1,
		eventAt,
	)

	if rowErr != nil {
		return
	}

	_ = signal.CrossSection.Observe(row)
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

	price := update.midPrice()

	if price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	volume := update.Volume

	if volume <= 0 {
		volume = price
	}

	pressure := update.pressure()

	row, rowErr := marketsection.NewSymbolRow(
		update.Symbol,
		price,
		0,
		volume,
		pressure,
		eventAt,
	)

	if rowErr != nil {
		return
	}

	_ = signal.CrossSection.Observe(row)
}

func (update tickerUpdate) midPrice() float64 {
	if update.Last > 0 {
		return update.Last
	}

	if update.Bid > 0 && update.Ask > update.Bid {
		return (update.Bid + update.Ask) / 2
	}

	return 0
}

func (update tickerUpdate) pressure() float64 {
	if update.BidQty <= 0 && update.AskQty <= 0 {
		return 1.0
	}

	return update.BidQty / (update.BidQty + update.AskQty)
}

package trader

import (
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
)

func (crypto *Crypto) sendWallet() {
	if crypto.wallet == nil {
		return
	}

	crypto.attachWalletMarks()
	crypto.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: crypto.wallet.Snapshot()})

	now := time.Now().UTC().Format(time.RFC3339Nano)

	for symbol, mark := range crypto.wallet.Marks {
		if mark <= 0 {
			continue
		}

		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "mark",
			"ts":     now,
			"symbol": symbol,
			"price":  mark,
		}})
	}
}

func (crypto *Crypto) attachWalletMarks() {
	if crypto.wallet == nil {
		return
	}

	marks := make(map[string]float64)

	for base, qty := range crypto.wallet.Inventory {
		if qty <= config.System.LiveInventoryEpsilon {
			continue
		}

		symbol := base + "/" + crypto.wallet.Currency
		mark := 0.0

		if crypto.predictions != nil {
			mark = crypto.predictions.LastPrice(symbol)
		}

		if mark <= 0 && crypto.portfolioRisk != nil {
			mark = crypto.portfolioRisk.Mark(symbol)
		}

		if mark <= 0 {
			continue
		}

		marks[symbol] = mark
	}

	crypto.wallet.Marks = marks
}

func (crypto *Crypto) observeTicker(row market.TickerRow) error {
	if crypto.wallet == nil || crypto.portfolioRisk == nil || row.Symbol == "" {
		return nil
	}

	price := row.Last

	if price <= 0 && row.Bid > 0 && row.Ask > 0 {
		price = (row.Bid + row.Ask) / 2
	}

	if price <= 0 {
		return nil
	}

	open := openSymbols(crypto.wallet)
	tracked := false

	for _, symbol := range open {
		if symbol != row.Symbol {
			continue
		}

		tracked = true
		crypto.portfolioRisk.ObserveSymbolAt(symbol, price, time.Now())
	}

	if !tracked {
		return nil
	}

	crypto.sendWallet()

	return nil
}

func (crypto *Crypto) observeOpenPrices(batch []engine.Measurement, now time.Time) {
	if crypto.wallet == nil || crypto.portfolioRisk == nil {
		return
	}

	batchPrices := make(map[string]float64)

	for _, measurement := range batch {
		if len(measurement.Pairs) == 0 {
			continue
		}

		price := anchorPrice(measurement)

		if price <= 0 {
			continue
		}

		batchPrices[measurement.Pairs[0].Wsname] = price
	}

	for _, symbol := range openSymbols(crypto.wallet) {
		price := batchPrices[symbol]

		if price <= 0 && crypto.predictions != nil {
			price = crypto.predictions.LastPrice(symbol)
		}

		if price <= 0 {
			continue
		}

		crypto.portfolioRisk.ObserveSymbolAt(symbol, price, now)
	}
}

func (crypto *Crypto) openCount() int {
	if crypto.wallet == nil {
		return 0
	}

	return activeSlotCount(crypto.wallet, crypto.restingEntries)
}

func (crypto *Crypto) updateEquity(now time.Time) {
	if crypto.wallet == nil {
		return
	}

	equity := crypto.wallet.MarkEquity(crypto.portfolioRisk.lastPrices)
	crypto.portfolioRisk.UpdateEquity(equity, now)
}

func anchorPrice(measurement engine.Measurement) float64 {
	if measurement.Last > 0 {
		return measurement.Last
	}

	if measurement.Bid > 0 && measurement.Ask > 0 {
		return (measurement.Bid + measurement.Ask) / 2
	}

	return 0
}

func perspectiveType(measurement engine.Measurement) engine.PerspectiveType {
	switch measurement.Type {
	case engine.Flow, engine.DepthFlow:
		return engine.PerspectiveFlow
	case engine.Basis, engine.LeadLag:
		return engine.PerspectiveCrossAsset
	case engine.Sentiment, engine.Causal:
		return engine.PerspectiveSentiment
	default:
		return engine.PerspectiveMicrostructure
	}
}

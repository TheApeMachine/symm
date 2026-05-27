package trader

import (
	"strings"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/order"
)

type restingEntry struct {
	Symbol     string
	OrderID    string
	LimitPrice float64
	Notional   float64
	PlacedAt   time.Time
}

func (crypto *Crypto) defendRestingEntries(batch []engine.Measurement) {
	if len(crypto.restingEntries) == 0 {
		return
	}

	for _, measurement := range batch {
		if !measurementAdverseToBid(measurement) {
			continue
		}

		if len(measurement.Pairs) == 0 {
			continue
		}

		symbol := measurement.Pairs[0].Wsname
		entry, ok := crypto.restingEntries[symbol]

		if !ok {
			continue
		}

		crypto.cancelRestingEntry(entry, measurement.Source)
	}
}

func measurementAdverseToBid(measurement engine.Measurement) bool {
	if measurement.Confidence < config.System.DefensiveOBIConfidence {
		return false
	}

	if measurement.Type == engine.Dump {
		return true
	}

	return measurement.Source == "depthflow" && measurement.Type == engine.DepthFlow
}

func (crypto *Crypto) cancelRestingEntry(entry restingEntry, reason string) {
	delete(crypto.restingEntries, entry.Symbol)

	if crypto.wallet != nil && entry.Notional > 0 {
		crypto.wallet.ReleaseEntryReservation(entry.Notional)
	}

	if entry.OrderID != "" {
		crypto.pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Send(&qpool.QValue[any]{
			Value: order.CancelOrder(entry.OrderID, ""),
		})
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":  "entry_canceled",
		"symbol": entry.Symbol,
		"reason": reason,
	}})
}

func (crypto *Crypto) enter(opportunity tradeOpportunity, slot float64) {
	measurement := opportunity.Measurement

	if crypto.wallet == nil || len(measurement.Pairs) == 0 || slot <= 0 {
		return
	}

	symbol := measurement.Pairs[0].Wsname
	last := anchorPrice(measurement)
	bid := measurement.Bid
	ask := measurement.Ask

	if last <= 0 {
		return
	}

	if bid <= 0 {
		bid = last
	}

	if ask <= 0 {
		ask = last
	}

	if slot < config.System.MinCostEUR {
		return
	}

	if config.System.UseMakerEntries {
		crypto.enterMaker(symbol, bid, ask, slot, measurement)
		return
	}

	crypto.enterTaker(symbol, last, bid, ask, slot, measurement)
}

func (crypto *Crypto) enterMaker(
	symbol string,
	bid, ask float64,
	slot float64,
	measurement engine.Measurement,
) {
	if err := crypto.wallet.ReserveEntry(slot); err != nil {
		return
	}

	if crypto.wallet.Type == CryptoWallet {
		crypto.restingEntries[symbol] = restingEntry{
			Symbol:     symbol,
			LimitPrice: bid,
			Notional:   slot,
			PlacedAt:   time.Now(),
		}

		crypto.pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Send(&qpool.QValue[any]{
			Value: order.LimitBuyBid(symbol, slot, bid, ""),
		})

		return
	}

	fillPrice := bid
	feePct := config.System.MakerFeePct

	if feePct <= 0 {
		feePct = crypto.wallet.FeePct
	}

	fee := slot * feePct / 100

	if err := crypto.wallet.SettleEntryReservation(slot, slot+fee); err != nil {
		crypto.wallet.ReleaseEntryReservation(slot)
		return
	}

	base := strings.Split(symbol, "/")[0]
	qty := (slot - fee) / fillPrice

	if qty <= 0 {
		return
	}

	crypto.wallet.Inventory[base] += qty
	crypto.wallet.RecordFill(base, qty, fillPrice)
	crypto.sendWallet()
}

func (crypto *Crypto) enterTaker(
	symbol string,
	last, bid, ask float64,
	slot float64,
	measurement engine.Measurement,
) {
	if crypto.wallet.Type == CryptoWallet {
		if err := crypto.wallet.ReserveEntry(slot); err != nil {
			return
		}

		crypto.pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Send(&qpool.QValue[any]{
			Value: order.MarketBuyCash(symbol, slot, 0, 0, ""),
		})

		return
	}

	if err := crypto.wallet.ReserveEntry(slot); err != nil {
		return
	}

	fillPrice := config.System.SlippageFill(
		last, bid, ask, "buy", config.System.SlippageBPS, slot, nil, nil,
	)
	cost := slot
	fee := cost * crypto.wallet.FeePct / 100

	if err := crypto.wallet.SettleEntryReservation(slot, cost+fee); err != nil {
		crypto.wallet.ReleaseEntryReservation(slot)
		return
	}

	base := strings.Split(symbol, "/")[0]
	qty := (cost - fee) / fillPrice

	if qty <= 0 {
		return
	}

	crypto.wallet.Inventory[base] += qty
	crypto.wallet.RecordFill(base, qty, fillPrice)
	crypto.sendWallet()
}

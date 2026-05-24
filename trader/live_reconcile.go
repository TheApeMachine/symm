package trader

import (
	"fmt"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/rest"
)

/*
ReconcileLive aligns wallet cash and inventory with Kraken and rebuilds open positions.
*/
func (portfolio *Portfolio) ReconcileLive(
	wallet *Wallet,
	balance *rest.Balance,
	pairIndex map[string]asset.Pair,
	journal *OrderJournal,
) error {
	if wallet == nil || balance == nil {
		return fmt.Errorf("wallet and balance are required")
	}

	if journal != nil {
		journal.LoadFromDisk()
	}

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	if quoteBalance, ok := balance.QuoteBalance(config.System.QuoteCurrency); ok {
		wallet.Balance = quoteBalance
	}

	wallet.ReservedEUR = 0
	wallet.Inventory = make(map[string]float64)

	for symbol, pair := range pairIndex {
		baseQty, ok := balance.AssetBalance(pair.Base)

		if !ok || baseQty <= config.System.LiveInventoryEpsilon {
			continue
		}

		wallet.Inventory[symbol] = baseQty
	}

	portfolio.reconcileStopOrdersLocked()

	for symbol, position := range portfolio.positions {
		if err := portfolio.verifyLiveInventoryLocked(symbol, position); err != nil {
			portfolio.haltLocked("inventory_mismatch:" + symbol)

			return err
		}
	}

	for symbol, baseQty := range wallet.Inventory {
		if _, open := portfolio.positions[symbol]; open {
			continue
		}

		entry, ok := journalOpenEntry(journal, symbol)

		if !ok {
			portfolio.haltLocked("orphan_inventory:" + symbol)

			return fmt.Errorf("orphan inventory for %s: %.8f base", symbol, baseQty)
		}

		position := portfolio.recoverPositionLocked(symbol, entry, baseQty)

		if position.StopOrderID == "" {
			portfolio.haltLocked("unprotected_position:" + symbol)

			return fmt.Errorf("recovered position missing stop for %s", symbol)
		}

		portfolio.positions[symbol] = position
	}

	portfolio.rehydratePaperStopsLocked()

	return nil
}

func (portfolio *Portfolio) verifyLiveInventoryLocked(symbol string, position *Position) error {
	if portfolio.wallet == nil || position == nil || position.Side != positionLong {
		return nil
	}

	available := portfolio.wallet.AvailableBase(symbol)
	epsilon := config.System.LiveInventoryEpsilon

	if available+epsilon < position.BaseQty {
		return fmt.Errorf(
			"inventory shortfall for %s: have %.8f need %.8f",
			symbol,
			available,
			position.BaseQty,
		)
	}

	return nil
}

func (portfolio *Portfolio) recoverPositionLocked(
	symbol string,
	entry OrderJournalEntry,
	baseQty float64,
) *Position {
	fillPrice := entry.FillPrice

	if fillPrice <= 0 {
		fillPrice = entry.NotionalEUR / baseQty
	}

	notional := spotProceedsEUR(baseQty, fillPrice)

	if notional <= 0 {
		notional = entry.NotionalEUR
	}

	return &Position{
		Symbol:      symbol,
		Side:        positionLong,
		EntryPrice:  fillPrice,
		FillPrice:   fillPrice,
		StopPrice:   initialStop(fillPrice, config.System.DefaultTrailPct, positionLong),
		PeakPrice:   fillPrice,
		NotionalEUR: notional,
		BaseQty:     baseQty,
		OrderID:     entry.OrderID,
		StopOrderID: entry.StopOrderID,
		OpenedAt:    entry.TS,
		TrailPct:    config.System.DefaultTrailPct,
	}
}

func journalOpenEntry(journal *OrderJournal, symbol string) (OrderJournalEntry, bool) {
	if journal == nil || symbol == "" {
		return OrderJournalEntry{}, false
	}

	open := false
	var lastEntry OrderJournalEntry

	for _, entry := range journal.Entries() {
		if entry.Symbol != symbol {
			continue
		}

		switch entry.Event {
		case "trade_enter", "trade_entered":
			open = true
			lastEntry = entry
		case "trade_exit":
			open = false
		}
	}

	if !open {
		return OrderJournalEntry{}, false
	}

	return lastEntry, true
}

func (portfolio *Portfolio) rehydratePaperStopsLocked() {
	paperBroker, ok := portfolio.broker.(*PaperBroker)

	if !ok {
		return
	}

	for _, position := range portfolio.positions {
		if position.StopOrderID == "" || position.StopPrice <= 0 {
			continue
		}

		paperBroker.restoreRestingStop(position.Symbol, position.StopOrderID, position.StopPrice)
	}
}

/*
ReconcileLive wires Kraken balances into the running crypto trader.
*/
func (crypto *Crypto) ReconcileLive(
	balance *rest.Balance,
	pairIndex map[string]asset.Pair,
) error {
	return crypto.portfolio.ReconcileLive(crypto.wallet, balance, pairIndex, crypto.orderJournal)
}

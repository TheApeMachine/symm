package trader

import (
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/wallet"
)

/*
sendWallet publishes the current wallet snapshot to ui clients.
*/
func (crypto *Crypto) sendWallet() {
	if crypto.wallet == nil {
		return
	}

	crypto.attachWalletMarks()
	snapshot := crypto.wallet.Snapshot()
	inventory := snapshot.Inventory
	avgEntry := snapshot.AvgEntry
	marks := snapshot.Marks

	if inventory == nil {
		inventory = map[string]float64{}
	}

	if avgEntry == nil {
		avgEntry = map[string]float64{}
	}

	if marks == nil {
		marks = map[string]float64{}
	}

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "wallet",
		"Currency":    snapshot.Currency,
		"Balance":     snapshot.Balance,
		"ReservedEUR": snapshot.ReservedEUR,
		"FeePct":      snapshot.FeePct,
		"Inventory":   inventory,
		"AvgEntry":    avgEntry,
		"Marks":       marks,
	}})

	now := time.Now().UTC().Format(time.RFC3339Nano)

	for symbol, mark := range snapshot.Marks {
		if mark <= 0 {
			continue
		}

		crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "mark",
			"ts":     now,
			"symbol": symbol,
			"price":  mark,
		}})
	}
}

/*
ResendWallet publishes the current wallet snapshot after the UI hub is listening.
*/
func (crypto *Crypto) ResendWallet() {
	crypto.sendWallet()
}

func (crypto *Crypto) attachWalletMarks() {
	if crypto.wallet == nil || crypto.forecasts == nil {
		return
	}

	inventory := crypto.wallet.InventoryCopy()
	marks := make(map[string]float64, len(inventory))

	for base, qty := range inventory {
		if qty <= config.System.LiveInventoryEpsilon {
			continue
		}

		symbol := base + "/" + crypto.wallet.Currency
		mark := crypto.forecasts.LastPrice(symbol)

		if mark <= 0 {
			continue
		}

		marks[symbol] = mark
	}

	crypto.wallet.SetMarks(marks)
}

func (crypto *Crypto) openCount() int {
	if crypto.wallet == nil {
		return 0
	}

	count := 0

	for _, qty := range crypto.wallet.InventoryCopy() {
		if qty > config.System.LiveInventoryEpsilon {
			count++
		}
	}

	return count
}

/*
recordEntryPnL informs the risk account about a fresh entry so the equity
high-water mark stays aligned with the wallet's mark-to-market view.
*/
func (crypto *Crypto) recordEntryPnL(symbol string, fillPrice float64) {
	if crypto.risk == nil || crypto.wallet == nil {
		return
	}

	crypto.risk.ObserveMark(symbol, fillPrice, time.Now())

	marks := map[string]float64{}

	for base, qty := range crypto.wallet.InventoryCopy() {
		if qty <= config.System.LiveInventoryEpsilon {
			continue
		}

		marks[base+"/"+crypto.wallet.Currency] = crypto.forecasts.LastPrice(base + "/" + crypto.wallet.Currency)
	}

	equity := crypto.wallet.MarkEquity(marks)
	crypto.risk.ApplyFillPnL(0, equity, time.Now())
}

/*
recordExitPnL books realized PnL into the daily accumulator. avgEntryBefore
is captured by the caller before the sell flattens inventory; reading the
wallet's AvgEntry here would always return 0 because Sell.FillPaper has
already cleared it via ZeroInventory. delta is qty × (exitPrice -
avgEntryBefore), which is realized PnL in quote currency for that leg of
the round-trip.
*/
func (crypto *Crypto) recordExitPnL(symbol string, qty, exitPrice, avgEntryBefore float64) {
	if crypto.risk == nil || crypto.wallet == nil {
		return
	}

	delta := (exitPrice - avgEntryBefore) * qty

	crypto.risk.ObserveMark(symbol, exitPrice, time.Now())

	marks := map[string]float64{}

	for held, q := range crypto.wallet.InventoryCopy() {
		if q <= config.System.LiveInventoryEpsilon {
			continue
		}

		marks[held+"/"+crypto.wallet.Currency] = crypto.forecasts.LastPrice(held + "/" + crypto.wallet.Currency)
	}

	equity := crypto.wallet.MarkEquity(marks)
	crypto.risk.ApplyFillPnL(delta, equity, time.Now())
}

/*
openSymbols returns the wsname for every base currently held. Used by the
risk gate to assemble the systemic-correlation candidate set.
*/
func (crypto *Crypto) openSymbols() []string {
	if crypto.wallet == nil {
		return nil
	}

	symbols := make([]string, 0)

	for base, qty := range crypto.wallet.InventoryCopy() {
		if qty <= config.System.LiveInventoryEpsilon {
			continue
		}

		symbols = append(symbols, base+"/"+crypto.wallet.Currency)
	}

	return symbols
}

func (crypto *Crypto) holdsSymbol(tradingWallet *wallet.Wallet, symbol string) bool {
	if tradingWallet == nil {
		return false
	}

	base := symbolBase(symbol)

	return tradingWallet.InventoryQty(base) > config.System.LiveInventoryEpsilon
}

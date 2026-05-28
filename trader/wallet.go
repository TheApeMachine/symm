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

	if inventory == nil {
		inventory = map[string]float64{}
	}

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "wallet",
		"Currency":    snapshot.Currency,
		"Balance":     snapshot.Balance,
		"ReservedEUR": snapshot.ReservedEUR,
		"FeePct":      snapshot.FeePct,
		"Inventory":   inventory,
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

	marks := make(map[string]float64)

	for base, qty := range crypto.wallet.Inventory {
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

	crypto.wallet.Marks = marks
}

func (crypto *Crypto) openCount() int {
	if crypto.wallet == nil {
		return 0
	}

	count := 0

	for _, qty := range crypto.wallet.Inventory {
		if qty > config.System.LiveInventoryEpsilon {
			count++
		}
	}

	return count
}

func (crypto *Crypto) holdsSymbol(tradingWallet *wallet.Wallet, symbol string) bool {
	if tradingWallet == nil {
		return false
	}

	base := symbolBase(symbol)

	return tradingWallet.Inventory[base] > config.System.LiveInventoryEpsilon
}

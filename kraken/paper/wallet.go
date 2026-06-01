package paper

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/user"
)

/*
Wallet holds spot balances for the simulated balances channel.
*/
type Wallet struct {
	mu       sync.Mutex
	quote    string
	balances map[string]float64
}

func NewWallet(quote string, initialCash float64) *Wallet {
	balances := make(map[string]float64)

	if initialCash > 0 {
		balances[quote] = initialCash
	}

	return &Wallet{
		quote:    quote,
		balances: balances,
	}
}

func (wallet *Wallet) Snapshot() []user.Balance {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	rows := make([]user.Balance, 0, len(wallet.balances))

	for asset, balance := range wallet.balances {
		if balance <= 0 {
			continue
		}

		rows = append(rows, snapshotRow(asset, balance))
	}

	return rows
}

func (wallet *Wallet) ApplyFill(
	symbol, side string,
	qty, price, fee float64,
	refID string,
) []user.Balance {
	base := baseAsset(symbol)
	quote := quoteAsset(symbol)
	cost := qty * price
	now := time.Now().UTC().Format(time.RFC3339Nano)

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if side == "buy" {
		wallet.balances[base] += qty
		wallet.balances[quote] -= cost + fee

		return []user.Balance{
			updateRow(base, qty, wallet.balances[base], 0, refID, now),
			updateRow(quote, -(cost + fee), wallet.balances[quote], fee, refID, now),
		}
	}

	wallet.balances[base] -= qty
	baseBalance := wallet.balances[base]

	if baseBalance <= 0 {
		delete(wallet.balances, base)
		baseBalance = 0
	}

	wallet.balances[quote] += cost - fee

	return []user.Balance{
		updateRow(base, -qty, baseBalance, 0, refID, now),
		updateRow(quote, cost-fee, wallet.balances[quote], fee, refID, now),
	}
}

func snapshotRow(asset string, balance float64) user.Balance {
	return user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Balance:    balance,
		Wallets: []user.BalanceWallet{{
			Balance: balance,
			Type:    "spot",
			ID:      "main",
		}},
	}
}

func updateRow(
	asset string,
	amount, balance, fee float64,
	refID, timestamp string,
) user.Balance {
	return user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Amount:     amount,
		Balance:    balance,
		Fee:        fee,
		LedgerID:   nextLedgerID(),
		RefID:      refID,
		Timestamp:  timestamp,
		Type:       "trade",
		Category:   "trade",
		WalletType: "spot",
		WalletID:   "main",
	}
}

func baseAsset(symbol string) string {
	if index := strings.IndexByte(symbol, '/'); index >= 0 {
		return symbol[:index]
	}

	return symbol
}

func nextLedgerID() string {
	buffer := make([]byte, 6)

	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("PAPER-%x", time.Now().UnixNano())
	}

	return strings.ToUpper(hex.EncodeToString(buffer))
}

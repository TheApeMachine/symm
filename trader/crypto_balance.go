package trader

import (
	"fmt"
	"maps"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	symmarket "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func (crypto *Crypto) ensureBalanceSnapshot() error {
	var snapshotErr error

	crypto.balanceOnce.Do(func() {
		snapshotErr = crypto.requestBalanceSnapshot()
	})

	return snapshotErr
}

func (crypto *Crypto) requestBalanceSnapshot() error {
	if viper.GetString("trading.model") != "paper" || crypto.pool == nil {
		return nil
	}

	if err := user.NewBalance(crypto.pool, nil); err != nil {
		return fmt.Errorf("trader/crypto: balance snapshot: %w", err)
	}

	return nil
}

func (crypto *Crypto) handleRaw(message *qpool.QValue[any]) error {
	if message == nil || message.Value == nil {
		return nil
	}

	if action, ok := message.Value.(perspectives.Action); ok {
		return crypto.handleAction(action)
	}

	envelope, ok := message.Value.(public.SocketMessage)

	if !ok {
		return nil
	}

	quote, err := symmarket.RequiredQuoteCurrency()

	if err != nil {
		return fmt.Errorf("trader/crypto: %w", err)
	}

	switch envelope.Channel {
	case public.ExecutionsChannel:
		activate.Once("trader/crypto:executions-channel")

		executions, err := user.DecodeExecutions(&envelope)

		if err != nil {
			return fmt.Errorf("trader/crypto: decode executions: %w", err)
		}

		for _, execution := range executions {
			if execution.ExecType == "rejected" || execution.OrderStatus == "rejected" {
				if execution.Symbol != "" {
					crypto.pendingOrders.Delete(execution.Symbol)
				}

				continue
			}

			if err := crypto.publishFill(execution); err != nil {
				return err
			}
		}
	case public.BalancesChannel:
		activate.Once("trader/crypto:balances-channel")
		trading.MarkDeskReady()

		balances, err := user.DecodeBalances(&envelope)

		if err != nil {
			return fmt.Errorf("trader/crypto: decode balances: %w", err)
		}

		for _, balance := range balances {
			if isQuoteAsset(balance.Asset, quote) {
				crypto.cash = balance.Balance
			} else if balance.Balance > 0 {
				crypto.inventory[balance.Asset] = balance.Balance
			} else {
				delete(crypto.inventory, balance.Asset)
			}
		}

		return crypto.sendWallet(crypto.cash, walletInventorySnapshot(crypto.inventory))
	}

	return nil
}

func walletInventorySnapshot(inventory map[string]float64) map[string]float64 {
	snapshot := make(map[string]float64, len(inventory))
	maps.Copy(snapshot, inventory)
	return snapshot
}

func (crypto *Crypto) resendWallet() error {
	return crypto.sendWallet(crypto.cash, walletInventorySnapshot(crypto.inventory))
}

func (crypto *Crypto) sendWallet(cash float64, inventory map[string]float64) error {
	quote, err := symmarket.RequiredQuoteCurrency()

	if err != nil {
		return fmt.Errorf("trader/crypto: wallet: %w", err)
	}

	avgEntry := make(map[string]float64, len(crypto.avgEntry))
	marks := make(map[string]float64, len(inventory))

	for base, price := range crypto.avgEntry {
		avgEntry[base] = price
	}

	for base, quantity := range inventory {
		if quantity <= 0 {
			continue
		}

		symbol := base + "/" + quote

		if mark, ok := crypto.marks[symbol]; ok && mark > 0 {
			marks[symbol] = mark
		}
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "wallet",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"Currency":  quote,
		"Balance":   cash,
		"Inventory": inventory,
		"AvgEntry":  avgEntry,
		"Marks":     marks,
	}})

	return nil
}

func isQuoteAsset(asset, quote string) bool {
	return asset == quote || asset == "Z"+quote
}

package market

import (
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
)

type pendingIntent struct {
	Side            trading.Side
	EntryConfidence float64
}

func (story *Story) startAccountSync() {
	if story == nil {
		return
	}

	story.accountSyncOnce.Do(func() {
		go story.syncAccountState()
	})
}

func (story *Story) syncAccountState() {
	for {
		row, receiveErr := story.bus.Receive(internal.ChannelRaw)

		if internal.IsShutdown(receiveErr) {
			return
		}

		if internal.ReportError(receiveErr) != nil || row == nil {
			continue
		}

		switch rawbus.TypeFrom(row.Type) {
		case rawbus.TypeBalances:
			balances, ok := row.Value.(user.Balances)

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"story: invalid balances",
					nil,
				))
				continue
			}

			story.applyBalance(balances)
		case rawbus.TypeExecutions:
			executions, err := rawbus.DecodeExecutions(row)

			if err != nil {
				errnie.Error(err)
				continue
			}

			for _, execution := range executions {
				story.applyExecution(execution)
			}
		}
	}
}

func (story *Story) submitAction(action *logic.Action) error {
	if err := rawbus.Send(story.bus, rawbus.TypeActions, action); err != nil {
		return err
	}

	story.markPendingIntent(action)

	return nil
}

func (story *Story) hasPendingIntent(action *logic.Action) bool {
	if story == nil || action == nil || story.pendingIntents == nil {
		return false
	}

	_, pending := story.pendingIntents.Load(pendingIntentKey(action.Side, action.Symbol))

	return pending
}

func (story *Story) markPendingIntent(action *logic.Action) {
	if story == nil || action == nil || story.pendingIntents == nil {
		return
	}

	if action.Side != trading.Buy && !action.Type.IsExit() {
		return
	}

	story.pendingIntents.Store(pendingIntentKey(action.Side, action.Symbol), pendingIntent{
		Side:            action.Side,
		EntryConfidence: action.EntryConfidence,
	})
}

func (story *Story) applyBalance(balances user.Balances) {
	if story == nil || story.holdings == nil {
		return
	}

	currency := balanceCurrency(balances)

	for base, quantity := range balances.Inventory {
		story.applyBalanceQuantity(base, currency, quantity)
	}

	for _, asset := range balances.Asset {
		story.applyBalanceAsset(asset, currency)
	}
}

func (story *Story) applyBalanceAsset(asset user.Balance, currency string) {
	assetName := strings.ToUpper(strings.TrimSpace(asset.Asset))

	if assetName == "" || isBalanceQuoteAsset(assetName, currency) {
		return
	}

	story.applyBalanceQuantity(assetName, currency, asset.Balance)
}

func (story *Story) applyBalanceQuantity(
	base string,
	currency string,
	quantity float64,
) {
	symbol := strings.ToUpper(strings.TrimSpace(base)) + "/" + currency

	if symbol == "/"+currency {
		return
	}

	if quantity <= 0 {
		story.holdings.SetPosition(symbol, 0, 0)
		story.clearPendingIntent(trading.Buy, symbol)
		story.clearPendingIntent(trading.Sell, symbol)
		return
	}

	confidence := story.pendingEntryConfidence(symbol)

	story.holdings.SetPosition(symbol, quantity, confidence)
	story.clearPendingIntent(trading.Buy, symbol)
}

func (story *Story) applyExecution(execution user.Execution) {
	if !isRejectedExecution(execution) {
		return
	}

	symbol := strings.ToUpper(strings.TrimSpace(execution.Symbol))

	if symbol == "" {
		return
	}

	side := trading.Side(strings.ToLower(strings.TrimSpace(execution.Side)))

	if side == trading.Buy || side == trading.Sell {
		story.clearPendingIntent(side, symbol)
		return
	}

	story.clearPendingIntent(trading.Buy, symbol)
	story.clearPendingIntent(trading.Sell, symbol)
}

func (story *Story) pendingEntryConfidence(symbol string) float64 {
	if story == nil || story.pendingIntents == nil {
		return 0
	}

	raw, ok := story.pendingIntents.Load(pendingIntentKey(trading.Buy, symbol))

	if !ok {
		return 0
	}

	pending, ok := raw.(pendingIntent)

	if !ok {
		return 0
	}

	return pending.EntryConfidence
}

func (story *Story) clearPendingIntent(side trading.Side, symbol string) {
	if story == nil || story.pendingIntents == nil {
		return
	}

	story.pendingIntents.Delete(pendingIntentKey(side, symbol))
}

func pendingIntentKey(side trading.Side, symbol string) string {
	return string(side) + ":" + strings.ToUpper(strings.TrimSpace(symbol))
}

func balanceCurrency(balances user.Balances) string {
	currency := strings.ToUpper(strings.TrimSpace(balances.Currency))

	if currency != "" {
		return currency
	}

	return "USD"
}

func isBalanceQuoteAsset(asset string, currency string) bool {
	return asset == currency || asset == "Z"+currency
}

func isRejectedExecution(execution user.Execution) bool {
	status := strings.ToLower(strings.TrimSpace(execution.OrderStatus))

	switch status {
	case "canceled", "cancelled", "expired", "rejected":
		return true
	default:
		return false
	}
}

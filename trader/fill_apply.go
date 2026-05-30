package trader

import (
	"fmt"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

func applyBuyFill(
	tradingWallet *wallet.Wallet,
	fill order.Fill,
	intent orderIntent,
) error {
	base := baseOf(fill.Symbol)
	cashDelta := broker.CashDeltaBuy(fill, tradingWallet.Currency)

	if err := tradingWallet.SettleEntryReservation(intent.notional, cashDelta); err != nil {
		return fmt.Errorf("settle entry reservation: %w", err)
	}

	if !tradingWallet.ApplyFill(fill.ExecKey, "buy", base, fill.Qty, fill.Price, cashDelta) {
		return fmt.Errorf("duplicate buy fill %s", fill.ExecKey)
	}

	return nil
}

func applySellFill(tradingWallet *wallet.Wallet, fill order.Fill) error {
	base := baseOf(fill.Symbol)
	cashDelta := broker.CashDeltaSell(fill, tradingWallet.Currency)

	if !tradingWallet.ApplyFill(fill.ExecKey, "sell", base, fill.Qty, fill.Price, cashDelta) {
		return fmt.Errorf("duplicate sell fill %s", fill.ExecKey)
	}

	return nil
}

func releaseEntryReservation(tradingWallet *wallet.Wallet, notional float64) {
	if notional > 0 {
		tradingWallet.ReleaseEntryReservation(notional)
	}
}

func handleRejectAck(
	session *orderSession,
	tradingWallet *wallet.Wallet,
	ack order.Ack,
) {
	clOrdID := ack.Result.ClOrdID

	intent, ok := session.intentFor(clOrdID)

	if !ok {
		return
	}

	session.dropIntent(clOrdID, intent.symbol)

	if intent.kind != "entry" {
		return
	}

	releaseEntryReservation(tradingWallet, intent.notional)
}

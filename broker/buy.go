package broker

import (
	"fmt"

	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
Buy is one spot long entry.
*/
type Buy struct {
	Symbol   string
	Notional float64
	Quote    Quote
}

/*
FillPaper simulates an immediate market buy.
*/
func (buy *Buy) FillPaper(tradingWallet *wallet.Wallet) (order.Fill, error) {
	if err := buy.validate(tradingWallet); err != nil {
		return order.Fill{}, err
	}

	if err := tradingWallet.ReserveEntry(buy.Notional); err != nil {
		return order.Fill{}, err
	}

	fillPrice, err := buy.Quote.FillPrice("buy", buy.Notional)

	if err != nil {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return order.Fill{}, err
	}

	fee := buy.Notional * tradingWallet.FeePct / 100

	if err := tradingWallet.SettleEntryReservation(buy.Notional, buy.Notional); err != nil {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return order.Fill{}, err
	}

	orderSymbol := Symbol(buy.Symbol)
	base := orderSymbol.BaseAsset()
	qty := (buy.Notional - fee) / fillPrice

	if qty <= 0 {
		return order.Fill{}, fmt.Errorf("invalid buy quantity for %s", buy.Symbol)
	}

	tradingWallet.AddInventory(base, qty, fillPrice)

	return order.Fill{
		OrderID: orderSymbol.PaperOrderID("buy"),
		Symbol:  buy.Symbol,
		Side:    "buy",
		Qty:     qty,
		Price:   fillPrice,
	}, nil
}

/*
SubmitLive reserves cash and routes a market buy.
*/
func (buy *Buy) SubmitLive(router *Router, tradingWallet *wallet.Wallet) error {
	if err := buy.validate(tradingWallet); err != nil {
		return err
	}

	if tradingWallet.Type != wallet.CryptoWallet {
		return fmt.Errorf("live buy requires crypto wallet")
	}

	if err := tradingWallet.ReserveEntry(buy.Notional); err != nil {
		return err
	}

	if err := router.Publish(order.MarketBuyCash(buy.Symbol, buy.Notional, 0, 0, "")); err != nil {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return err
	}

	return nil
}

func (buy *Buy) validate(tradingWallet *wallet.Wallet) error {
	if tradingWallet == nil {
		return fmt.Errorf("wallet is required")
	}

	if buy.Symbol == "" || buy.Notional <= 0 {
		return fmt.Errorf("invalid buy")
	}

	return nil
}

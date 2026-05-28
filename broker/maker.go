package broker

import (
	"fmt"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
Maker is one resting limit bid entry.
*/
type Maker struct {
	Symbol     string
	LimitPrice float64
	Notional   float64
	OrderID    string
}

/*
FillPaper settles one maker entry at its limit price.
*/
func (maker *Maker) FillPaper(tradingWallet *wallet.Wallet) (order.Fill, error) {
	if err := maker.validate(tradingWallet); err != nil {
		return order.Fill{}, err
	}

	if tradingWallet.Type != wallet.PaperWallet {
		return order.Fill{}, fmt.Errorf("maker fill is paper-only")
	}

	feePct := config.System.MakerFeePct

	if feePct <= 0 {
		feePct = tradingWallet.FeePct
	}

	fee := maker.Notional * feePct / 100

	if err := tradingWallet.SettleEntryReservation(maker.Notional, maker.Notional); err != nil {
		return order.Fill{}, err
	}

	base := baseAsset(maker.Symbol)
	qty := (maker.Notional - fee) / maker.LimitPrice

	if qty <= 0 {
		return order.Fill{}, fmt.Errorf("invalid maker quantity for %s", maker.Symbol)
	}

	tradingWallet.Inventory[base] += qty
	tradingWallet.RecordFill(base, qty, maker.LimitPrice)

	return order.Fill{
		OrderID: paperOrderID("maker", maker.Symbol),
		Symbol:  maker.Symbol,
		Side:    "buy",
		Qty:     qty,
		Price:   maker.LimitPrice,
	}, nil
}

/*
SubmitLive reserves cash and posts a limit bid.
*/
func (maker *Maker) SubmitLive(router *Router, tradingWallet *wallet.Wallet) error {
	if err := maker.validate(tradingWallet); err != nil {
		return err
	}

	if err := tradingWallet.ReserveEntry(maker.Notional); err != nil {
		return err
	}

	if tradingWallet.Type != wallet.CryptoWallet {
		return nil
	}

	if err := router.Publish(order.LimitBuyBid(maker.Symbol, maker.Notional, maker.LimitPrice, "")); err != nil {
		tradingWallet.ReleaseEntryReservation(maker.Notional)

		return err
	}

	return nil
}

/*
Cancel releases reservation and cancels a live order when present.
*/
func (maker *Maker) Cancel(router *Router, tradingWallet *wallet.Wallet) error {
	if tradingWallet != nil && maker.Notional > 0 {
		tradingWallet.ReleaseEntryReservation(maker.Notional)
	}

	if maker.OrderID == "" || router == nil {
		return nil
	}

	return router.Publish(order.CancelOrder(maker.OrderID, ""))
}

func (maker *Maker) validate(tradingWallet *wallet.Wallet) error {
	if tradingWallet == nil {
		return fmt.Errorf("wallet is required")
	}

	if maker.Symbol == "" || maker.Notional <= 0 || maker.LimitPrice <= 0 {
		return fmt.Errorf("invalid maker")
	}

	return nil
}

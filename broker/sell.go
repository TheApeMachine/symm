package broker

import (
	"fmt"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
Sell closes one spot long.
*/
type Sell struct {
	Symbol string
	Quote  Quote
}

/*
FillPaper simulates an immediate market sell of the full position.
*/
func (sell *Sell) FillPaper(tradingWallet *wallet.Wallet) (order.Fill, error) {
	if tradingWallet == nil {
		return order.Fill{}, fmt.Errorf("wallet is required")
	}

	if sell.Symbol == "" {
		return order.Fill{}, fmt.Errorf("invalid sell")
	}

	orderSymbol := Symbol(sell.Symbol)
	base := orderSymbol.BaseAsset()
	qty := tradingWallet.Inventory[base]

	if qty <= config.System.LiveInventoryEpsilon {
		return order.Fill{}, nil
	}

	last, _, _, err := sell.Quote.complete()

	if err != nil {
		return order.Fill{}, err
	}

	fillPrice, err := sell.Quote.FillPrice("sell", qty*last)

	if err != nil {
		return order.Fill{}, err
	}

	revenue := qty * fillPrice
	fee := revenue * tradingWallet.FeePct / 100

	tradingWallet.Inventory[base] = 0
	tradingWallet.ClearPosition(base)
	tradingWallet.Balance += revenue - fee

	return order.Fill{
		OrderID: orderSymbol.PaperOrderID("sell"),
		Symbol:  sell.Symbol,
		Side:    "sell",
		Qty:     qty,
		Price:   fillPrice,
	}, nil
}

/*
SubmitLive routes a market sell for the full position.
*/
func (sell *Sell) SubmitLive(router *Router, tradingWallet *wallet.Wallet) error {
	if tradingWallet == nil {
		return fmt.Errorf("wallet is required")
	}

	if sell.Symbol == "" {
		return fmt.Errorf("invalid sell")
	}

	if tradingWallet.Type != wallet.CryptoWallet {
		return fmt.Errorf("live sell requires crypto wallet")
	}

	orderSymbol := Symbol(sell.Symbol)
	base := orderSymbol.BaseAsset()
	qty := tradingWallet.Inventory[base]

	if qty <= config.System.LiveInventoryEpsilon {
		return nil
	}

	return router.Publish(order.MarketSellBase(sell.Symbol, qty, ""))
}

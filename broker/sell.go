package broker

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
Sell closes one spot long.
*/
type Sell struct {
	Symbol         string
	Quote          Quote
	LotDecimals    int
	HasLotDecimals bool
	FeePct         float64 // real per-pair taker fee; falls back to wallet.FeePct when <= 0
	ClOrdID        string
	Execution      config.ExecutionScope
}

/*
SubmitPaper validates a sell and assigns a client order id without filling.
*/
func (sell *Sell) SubmitPaper(tradingWallet *wallet.Wallet) (string, error) {
	if tradingWallet == nil {
		return "", fmt.Errorf("wallet is required")
	}

	if sell.Symbol == "" {
		return "", fmt.Errorf("invalid sell")
	}

	if sell.ClOrdID == "" {
		clOrdID, err := order.NextClOrdID()

		if err != nil {
			return "", fmt.Errorf("generate cl_ord_id: %w", err)
		}

		sell.ClOrdID = clOrdID
	}

	if err := ShouldRejectPaperOrder(sell.executionScope()); err != nil {
		return sell.ClOrdID, err
	}

	return sell.ClOrdID, nil
}

/*
BuildPaperFill prices a full-position sell for the paper execution simulator.
*/
func (sell *Sell) BuildPaperFill(tradingWallet *wallet.Wallet) (order.Fill, error) {
	if tradingWallet == nil {
		return order.Fill{}, fmt.Errorf("wallet is required")
	}

	if sell.Symbol == "" {
		return order.Fill{}, fmt.Errorf("invalid sell")
	}

	orderSymbol := Symbol(sell.Symbol)
	base := orderSymbol.BaseAsset()
	qty := tradingWallet.InventoryQty(base)

	if qty <= sell.executionScope().LiveInventoryEpsilon {
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
	fee := revenue * feeOr(sell.FeePct, tradingWallet.FeePct) / 100

	return order.Fill{
		OrderID: orderSymbol.PaperOrderID("sell"),
		ClOrdID: sell.ClOrdID,
		Symbol:  sell.Symbol,
		Side:    "sell",
		Qty:     qty,
		Price:   fillPrice,
		Fee:     fee,
		FeeCcy:  tradingWallet.Currency,
		ExecKey: "paper-" + sell.ClOrdID,
	}, nil
}

/*
FillPaper simulates an immediate market sell of the full position. Inventory
is read by atomically zeroing the slot — the quantity returned by
ZeroInventory is the one used for sizing, fee accounting, and the published
Fill, so a concurrent AddInventory between a stale read and the zero cannot
leave dangling base.
*/
func (sell *Sell) FillPaper(tradingWallet *wallet.Wallet) (order.Fill, error) {
	if _, err := sell.SubmitPaper(tradingWallet); err != nil {
		return order.Fill{}, err
	}

	fill, err := sell.BuildPaperFill(tradingWallet)

	if err != nil || fill.Qty <= 0 {
		return fill, err
	}

	base := Symbol(sell.Symbol).BaseAsset()
	consumed := tradingWallet.ZeroInventory(base)

	if consumed <= config.System.LiveInventoryEpsilon {
		return order.Fill{}, nil
	}

	fill.Qty = consumed
	proceeds := CashDeltaSell(fill, tradingWallet.Currency)
	tradingWallet.CreditBalance(proceeds)

	return fill, nil
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
	qty := tradingWallet.InventoryQty(base)

	if qty <= sell.executionScope().LiveInventoryEpsilon {
		return nil
	}

	lotDecimals, ok := sell.liveLotDecimals(base, tradingWallet)

	if !ok {
		return fmt.Errorf("lot decimals required for live sell: %s", sell.Symbol)
	}

	roundedQty, err := roundBaseQuantity(qty, lotDecimals)

	if err != nil {
		return err
	}

	if roundedQty <= config.System.LiveInventoryEpsilon {
		return nil
	}

	if sell.ClOrdID == "" {
		clOrdID, err := order.NextClOrdID()

		if err != nil {
			return fmt.Errorf("generate cl_ord_id: %w", err)
		}

		sell.ClOrdID = clOrdID
	}

	req := order.MarketSellBase(sell.Symbol, roundedQty, "")
	req.Params.ClOrdID = sell.ClOrdID

	return router.Publish(req)
}

func (sell *Sell) liveLotDecimals(base string, tradingWallet *wallet.Wallet) (int, bool) {
	if sell.HasLotDecimals {
		return sell.LotDecimals, true
	}

	binding, ok := tradingWallet.PositionBindingFor(base)

	if !ok || !binding.HasLotDecimals {
		return 0, false
	}

	return binding.LotDecimals, true
}

func roundBaseQuantity(qty float64, decimals int) (float64, error) {
	return roundDownPositive(qty, decimals, "quantity")
}

func roundQuotePrice(price float64, decimals int) (float64, error) {
	return roundDownPositive(price, decimals, "price")
}

func (sell *Sell) executionScope() config.ExecutionScope {
	if sell != nil && sell.Execution.QuoteCurrency != "" {
		return sell.Execution
	}

	if config.Runtime != nil {
		return config.Runtime.Execution
	}

	return config.ExecutionScopeFrom(config.System)
}

func roundDownPositive(value float64, decimals int, label string) (float64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", label)
	}

	if decimals < 0 {
		return 0, fmt.Errorf("%s decimals must be non-negative", label)
	}

	multiplier := math.Pow10(decimals)

	if multiplier <= 0 || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("invalid %s decimals: %d", label, decimals)
	}

	return math.Floor(value*multiplier) / multiplier, nil
}

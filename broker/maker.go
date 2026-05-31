package broker

import (
	"fmt"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
Maker is one resting limit bid entry. OrderID is populated from the live
exchange ack so Cancel can address the exchange's own identifier.
*/
type Maker struct {
	Symbol           string
	LimitPrice       float64
	Notional         float64
	OrderID          string
	ClOrdID          string
	PriceDecimals    int
	HasPriceDecimals bool
	FeePct           float64 // real per-pair maker fee; falls back to config/wallet when <= 0
}

/*
SubmitPaper reserves cash for one resting maker bid without filling.
*/
func (maker *Maker) SubmitPaper(tradingWallet *wallet.Wallet) (string, error) {
	if err := maker.validate(tradingWallet); err != nil {
		return "", err
	}

	if tradingWallet.Type != wallet.PaperWallet {
		return "", fmt.Errorf("paper maker requires paper wallet")
	}

	if err := tradingWallet.ReserveEntry(maker.Notional); err != nil {
		return "", err
	}

	if maker.ClOrdID == "" {
		clOrdID, err := order.NextClOrdID()

		if err != nil {
			tradingWallet.ReleaseEntryReservation(maker.Notional)

			return "", fmt.Errorf("generate cl_ord_id: %w", err)
		}

		maker.ClOrdID = clOrdID
	}

	if err := ShouldRejectPaperOrder(); err != nil {
		tradingWallet.ReleaseEntryReservation(maker.Notional)

		return maker.ClOrdID, err
	}

	return maker.ClOrdID, nil
}

/*
BuildPaperFill prices one maker fill once sell-aggressor volume clears the queue.
The desk settles through handleOrderFill like taker paper orders.
*/
func (maker *Maker) BuildPaperFill(queue MakerQueueContext) (order.Fill, error) {
	if maker == nil || maker.Symbol == "" || maker.Notional <= 0 || maker.LimitPrice <= 0 {
		return order.Fill{}, fmt.Errorf("invalid maker")
	}

	if maker.ClOrdID == "" {
		return order.Fill{}, fmt.Errorf("maker cl_ord_id is required")
	}

	feePct := maker.FeePct

	if feePct <= 0 {
		feePct = config.System.MakerFeePct
	}

	effectivePrice := maker.LimitPrice * (1 + float64(config.System.AdverseSelectionBPS)/10000)
	fee := maker.Notional * feePct / 100
	orderBaseQty := (maker.Notional - fee) / effectivePrice

	if !MakerFillReady(queue, maker.LimitPrice, orderBaseQty) {
		return order.Fill{}, ErrMakerQueueNotReady
	}

	if orderBaseQty <= 0 {
		return order.Fill{}, fmt.Errorf("invalid maker quantity for %s", maker.Symbol)
	}

	orderSymbol := Symbol(maker.Symbol)

	return order.Fill{
		OrderID: orderSymbol.PaperOrderID("maker"),
		ClOrdID: maker.ClOrdID,
		Symbol:  maker.Symbol,
		Side:    "buy",
		Qty:     orderBaseQty,
		Price:   effectivePrice,
		Fee:     fee,
		FeeCcy:  config.System.QuoteCurrency,
		ExecKey: "paper-" + maker.ClOrdID,
	}, nil
}

/*
FillPaper runs SubmitPaper and BuildPaperFill, then settles immediately for tests.
*/
func (maker *Maker) FillPaper(tradingWallet *wallet.Wallet, queue MakerQueueContext) (order.Fill, error) {
	return maker.fillPaperSettled(tradingWallet, queue)
}

func (maker *Maker) fillPaperSettled(tradingWallet *wallet.Wallet, queue MakerQueueContext) (order.Fill, error) {
	clOrdID, err := maker.SubmitPaper(tradingWallet)

	if err != nil {
		return order.Fill{}, err
	}

	fill, buildErr := maker.BuildPaperFill(queue)

	if buildErr != nil {
		tradingWallet.ReleaseEntryReservation(maker.Notional)

		return order.Fill{}, buildErr
	}

	if settleErr := tradingWallet.SettleEntryReservation(maker.Notional, maker.Notional); settleErr != nil {
		return order.Fill{}, settleErr
	}

	base := Symbol(maker.Symbol).BaseAsset()
	tradingWallet.AddInventoryWithCost(base, fill.Qty, maker.Notional)

	fill.ClOrdID = clOrdID

	return fill, nil
}

/*
SubmitLive reserves cash and posts a limit bid.
*/
func (maker *Maker) SubmitLive(router *Router, tradingWallet *wallet.Wallet) error {
	if err := maker.validate(tradingWallet); err != nil {
		return err
	}

	limitPrice := maker.LimitPrice

	if tradingWallet.Type == wallet.CryptoWallet {
		if !maker.HasPriceDecimals {
			return fmt.Errorf("price decimals required for live maker: %s", maker.Symbol)
		}

		roundedPrice, err := roundQuotePrice(maker.LimitPrice, maker.PriceDecimals)

		if err != nil {
			return err
		}

		limitPrice = roundedPrice
	}

	if err := tradingWallet.ReserveEntry(maker.Notional); err != nil {
		return err
	}

	if tradingWallet.Type != wallet.CryptoWallet {
		return nil
	}

	if maker.ClOrdID == "" {
		clOrdID, err := order.NextClOrdID()

		if err != nil {
			tradingWallet.ReleaseEntryReservation(maker.Notional)

			return fmt.Errorf("generate cl_ord_id: %w", err)
		}

		maker.ClOrdID = clOrdID
	}

	req := order.LimitBuyBid(maker.Symbol, maker.Notional, limitPrice, "")
	// LimitBuyBid stamps the token only; ClOrdID is set directly so the
	// path matches Buy.SubmitLive (which constructs MarketBuyCash and then
	// sets ClOrdID on the returned Request) and so reconcile-by-cl-ord-id
	// works on the ack path.
	req.Params.ClOrdID = maker.ClOrdID

	if err := router.Publish(req); err != nil {
		tradingWallet.ReleaseEntryReservation(maker.Notional)

		return err
	}

	return nil
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

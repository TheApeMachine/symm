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
}

/*
FillPaper settles one maker entry at its limit price. Cost basis recorded on
the wallet is the full notional (which absorbs the maker fee), not the limit
price, so realized PnL accounts for fees on both sides of every round trip.
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

	orderSymbol := Symbol(maker.Symbol)
	base := orderSymbol.BaseAsset()
	qty := (maker.Notional - fee) / maker.LimitPrice

	if qty <= 0 {
		return order.Fill{}, fmt.Errorf("invalid maker quantity for %s", maker.Symbol)
	}

	tradingWallet.AddInventoryWithCost(base, qty, maker.Notional)

	return order.Fill{
		OrderID: orderSymbol.PaperOrderID("maker"),
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

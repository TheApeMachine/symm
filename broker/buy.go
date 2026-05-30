package broker

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
Buy is one spot long entry. StopPrice / LimitBelowStop, when both positive,
attach a one-triggers-other stop-loss-limit on live submission.
*/
type Buy struct {
	Symbol         string
	Notional       float64
	Quote          Quote
	StopPrice      float64
	LimitBelowStop float64
	ClOrdID        string
	FeePct         float64 // real per-pair taker fee; falls back to wallet.FeePct when <= 0
}

/*
FillPaper simulates an immediate market buy. Cost basis recorded in the wallet
is the full cash spent (Notional, which absorbs the taker fee) divided by the
acquired quantity. This is what realized PnL and stop-distance computations
read back from AvgEntryFor.
*/
func (buy *Buy) FillPaper(tradingWallet *wallet.Wallet) (order.Fill, error) {
	if err := buy.validate(tradingWallet); err != nil {
		return order.Fill{}, err
	}

	if err := buy.PreflightGates(); err != nil {
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

	if err := buy.preflightFill(fillPrice); err != nil {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return order.Fill{}, err
	}

	fee := buy.Notional * feeOr(buy.FeePct, tradingWallet.FeePct) / 100

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

	tradingWallet.AddInventoryWithCost(base, qty, buy.Notional)

	return order.Fill{
		OrderID: orderSymbol.PaperOrderID("buy"),
		Symbol:  buy.Symbol,
		Side:    "buy",
		Qty:     qty,
		Price:   fillPrice,
	}, nil
}

/*
SubmitLive reserves cash and routes a market buy. StopPrice / LimitBelowStop,
when both set, attach an OTO stop-loss-limit on the primary fill so a gap
move still trips the exchange-side stop without depending on local pulse.
*/
func (buy *Buy) SubmitLive(router *Router, tradingWallet *wallet.Wallet) error {
	if err := buy.validate(tradingWallet); err != nil {
		return err
	}

	if tradingWallet.Type != wallet.CryptoWallet {
		return fmt.Errorf("live buy requires crypto wallet")
	}

	if err := buy.PreflightGates(); err != nil {
		return err
	}

	if err := tradingWallet.ReserveEntry(buy.Notional); err != nil {
		return err
	}

	if buy.ClOrdID == "" {
		clOrdID, err := order.NextClOrdID()

		if err != nil {
			tradingWallet.ReleaseEntryReservation(buy.Notional)

			return fmt.Errorf("generate cl_ord_id: %w", err)
		}

		buy.ClOrdID = clOrdID
	}

	req := order.MarketBuyCash(
		buy.Symbol, buy.Notional, buy.StopPrice, buy.LimitBelowStop, "",
	)
	req.Params.ClOrdID = buy.ClOrdID

	if err := router.Publish(req); err != nil {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return err
	}

	return nil
}

/*
PreflightGates enforces the pre-trade safety net configured globally:
SnapshotFreshnessTTL on the quote, MaxSpreadBPS on bid/ask spread, and a basic
sanity check against zero or inverted markets. MaxEntrySlippageBPS is checked
against the projected fillPrice in preflightFill once the quote has been
consulted.
*/
func (buy *Buy) PreflightGates() error {
	if buy == nil {
		return fmt.Errorf("nil buy")
	}

	cfg := config.System

	if cfg.SnapshotFreshnessTTL > 0 && !buy.Quote.At.IsZero() {
		if age := time.Since(buy.Quote.At); age > cfg.SnapshotFreshnessTTL {
			return fmt.Errorf("quote stale: %s > %s", age, cfg.SnapshotFreshnessTTL)
		}
	}

	if cfg.MaxSpreadBPS > 0 && buy.Quote.Bid > 0 && buy.Quote.Ask > 0 {
		mid := (buy.Quote.Bid + buy.Quote.Ask) / 2

		if mid > 0 {
			spreadBPS := (buy.Quote.Ask - buy.Quote.Bid) / mid * 10000

			if spreadBPS > cfg.MaxSpreadBPS {
				return fmt.Errorf("spread %.2f bps > MaxSpreadBPS %.2f", spreadBPS, cfg.MaxSpreadBPS)
			}
		}
	}

	return nil
}

func (buy *Buy) preflightFill(fillPrice float64) error {
	cfg := config.System

	if cfg.MaxEntrySlippageBPS <= 0 {
		return nil
	}

	last := buy.Quote.Last

	if last <= 0 {
		return nil
	}

	slipBPS := math.Abs(fillPrice-last) / last * 10000

	if slipBPS > cfg.MaxEntrySlippageBPS {
		return fmt.Errorf(
			"projected slippage %.2f bps > MaxEntrySlippageBPS %.2f",
			slipBPS, cfg.MaxEntrySlippageBPS,
		)
	}

	return nil
}

/*
feeOr returns the per-trade fee percent when one was supplied (the real
per-pair fee threaded in by the trader), otherwise the wallet's flat fallback.
*/
func feeOr(perTrade, fallback float64) float64 {
	if perTrade > 0 {
		return perTrade
	}

	return fallback
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

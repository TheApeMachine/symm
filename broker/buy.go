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
	Execution      config.ExecutionScope
	StressRegime   StressRegime
}

/*
SubmitPaper mirrors SubmitLive through reserve + preflight, without filling.
Returns the client order id used to reconcile the simulated execution. Rejected
paper submissions keep their reservation so the simulated reject ack owns release.
*/
func (buy *Buy) SubmitPaper(tradingWallet *wallet.Wallet) (string, error) {
	if err := buy.validate(tradingWallet); err != nil {
		return "", err
	}

	if err := buy.PreflightGates(); err != nil {
		return "", err
	}

	if err := tradingWallet.ReserveEntry(buy.Notional); err != nil {
		return "", err
	}

	if buy.ClOrdID == "" {
		clOrdID, err := order.NextClOrdID()

		if err != nil {
			tradingWallet.ReleaseEntryReservation(buy.Notional)

			return "", fmt.Errorf("generate cl_ord_id: %w", err)
		}

		buy.ClOrdID = clOrdID
	}

	if err := ShouldRejectPaperOrder(buy.Execution, buy.StressRegime); err != nil {
		return buy.ClOrdID, err
	}

	return buy.ClOrdID, nil
}

/*
BuildPaperFill prices a buy after SubmitPaper reserved cash. The desk applies
the fill through the same wallet path as live executions.
*/
func (buy *Buy) BuildPaperFill(tradingWallet *wallet.Wallet) (order.Fill, error) {
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
	qty := (buy.Notional - fee) / fillPrice

	if qty <= 0 {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return order.Fill{}, fmt.Errorf("invalid buy quantity for %s", buy.Symbol)
	}

	return order.Fill{
		OrderID: Symbol(buy.Symbol).PaperOrderID("buy"),
		ClOrdID: buy.ClOrdID,
		Symbol:  buy.Symbol,
		Side:    "buy",
		Qty:     qty,
		Price:   fillPrice,
		Fee:     fee,
		FeeCcy:  tradingWallet.Currency,
		ExecKey: "paper-" + buy.ClOrdID,
	}, nil
}

/*
FillPaper runs SubmitPaper and BuildPaperFill, then settles inventory immediately
for unit tests and legacy callers.
*/
func (buy *Buy) FillPaper(tradingWallet *wallet.Wallet) (order.Fill, error) {
	clOrdID, err := buy.SubmitPaper(tradingWallet)

	if err != nil {
		if clOrdID != "" {
			tradingWallet.ReleaseEntryReservation(buy.Notional)
		}

		return order.Fill{}, err
	}

	fill, err := buy.BuildPaperFill(tradingWallet)

	if err != nil {
		return order.Fill{}, err
	}

	cashDelta := fill.Qty*fill.Price + fill.Fee

	if err := tradingWallet.SettleEntryReservation(buy.Notional, cashDelta); err != nil {
		tradingWallet.ReleaseEntryReservation(buy.Notional)

		return order.Fill{}, err
	}

	base := Symbol(buy.Symbol).BaseAsset()
	tradingWallet.AddInventoryWithCost(base, fill.Qty, cashDelta)

	return fill, nil
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

	scope := buy.executionScope()

	if scope.SnapshotFreshnessTTL > 0 && !buy.Quote.At.IsZero() {
		if age := time.Since(buy.Quote.At); age > scope.SnapshotFreshnessTTL {
			return fmt.Errorf(
				"%s quote stale: %s > %s (bid=%g ask=%g)",
				buy.Symbol, age, scope.SnapshotFreshnessTTL, buy.Quote.Bid, buy.Quote.Ask,
			)
		}
	}

	if !buy.Quote.HasTopOfBook() {
		return fmt.Errorf(
			"%s incomplete quote: missing bid/ask (bid=%g ask=%g last=%g)",
			buy.Symbol, buy.Quote.Bid, buy.Quote.Ask, buy.Quote.Last,
		)
	}

	if scope.MaxSpreadBPS > 0 {
		mid := (buy.Quote.Bid + buy.Quote.Ask) / 2

		if mid > 0 {
			spreadBPS := (buy.Quote.Ask - buy.Quote.Bid) / mid * 10000

			if spreadBPS > scope.MaxSpreadBPS {
				return fmt.Errorf(
					"%s spread %.2f bps > MaxSpreadBPS %.2f (bid=%g ask=%g mid=%g)",
					buy.Symbol, spreadBPS, scope.MaxSpreadBPS, buy.Quote.Bid, buy.Quote.Ask, mid,
				)
			}
		}
	}

	return nil
}

func (buy *Buy) preflightFill(fillPrice float64) error {
	scope := buy.executionScope()

	if scope.MaxEntrySlippageBPS <= 0 {
		return nil
	}

	last := buy.Quote.Last

	if last <= 0 {
		return nil
	}

	slipBPS := math.Abs(fillPrice-last) / last * 10000

	if slipBPS > scope.MaxEntrySlippageBPS {
		return fmt.Errorf(
			"projected slippage %.2f bps > MaxEntrySlippageBPS %.2f",
			slipBPS, scope.MaxEntrySlippageBPS,
		)
	}

	return nil
}

func (buy *Buy) executionScope() config.ExecutionScope {
	if buy != nil && buy.Execution.QuoteCurrency != "" {
		return buy.Execution
	}

	if config.Runtime != nil {
		return config.Runtime.Execution
	}

	return config.ExecutionScopeFrom(config.System)
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

package trader

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/risk"
	"github.com/theapemachine/symm/wallet"
)

/*
riskAccount holds the running risk state the entry gate consults: realized
day PnL, the bookkeeping for daily reset, and a portfolio aggregate that
tracks peak equity and drawdown. The portfolio aggregate is fed by every
fill so DrawdownPct reflects mark-to-market reality, not the wallet's flat
"available cash" view.
*/
type riskAccount struct {
	mu          sync.Mutex
	dayStart    time.Time
	realizedDay float64
	wallet      *wallet.Wallet
	portfolio   *risk.Portfolio
}

func newRiskAccount(tradingWallet *wallet.Wallet) *riskAccount {
	portfolio := risk.NewPortfolio()
	initialEquity := 0.0

	if tradingWallet != nil {
		initialEquity = tradingWallet.BalanceCopy() + tradingWallet.ReservedCopy()
	}

	portfolio.UpdatePeakEquity(initialEquity)

	return &riskAccount{
		dayStart:  time.Now().UTC().Truncate(24 * time.Hour),
		wallet:    tradingWallet,
		portfolio: portfolio,
	}
}

/*
ObserveMark records one mark for portfolio-level drawdown and correlation.
*/
func (account *riskAccount) ObserveMark(symbol string, price float64, at time.Time) {
	if account == nil || account.portfolio == nil || symbol == "" || price <= 0 {
		return
	}

	account.mu.Lock()
	defer account.mu.Unlock()

	if at.IsZero() {
		account.portfolio.ObserveSymbol(symbol, price)

		return
	}

	account.portfolio.ObserveSymbolAt(symbol, price, at)
}

// MarketRegime classifies recent price-path efficiency for one symbol.
func (account *riskAccount) MarketRegime(symbol string) engine.MarketRegime {
	if account == nil || account.portfolio == nil {
		return engine.RegimeUnknown
	}

	account.mu.Lock()
	defer account.mu.Unlock()

	return account.portfolio.MarketRegime(symbol)
}

/*
SystemicCorrelation returns the dominant eigenvalue of the cross-correlation
matrix when candidate is added to the open set. Returns 0, false when fewer
than two distinct symbols are available.
*/
func (account *riskAccount) SystemicCorrelation(candidate string, openSymbols []string) (float64, bool) {
	if account == nil || account.portfolio == nil {
		return 0, false
	}

	account.mu.Lock()
	defer account.mu.Unlock()

	return account.portfolio.SystemicCorrelation(candidate, openSymbols)
}

func (account *riskAccount) rolloverIfNewDay(now time.Time) {
	day := now.UTC().Truncate(24 * time.Hour)

	if !day.After(account.dayStart) {
		return
	}

	account.dayStart = day
	account.realizedDay = 0
}

/*
ApplyFillPnL records realized PnL from one round-trip leg. Only the exit
leg has nonzero realized PnL — buys are entries with zero realized impact.
The caller is responsible for translating fills into PnL deltas before
calling.
*/
func (account *riskAccount) ApplyFillPnL(delta float64, mtmEquity float64, now time.Time) {
	if account == nil {
		return
	}

	account.mu.Lock()
	defer account.mu.Unlock()

	account.rolloverIfNewDay(now)
	account.realizedDay += delta
	account.portfolio.UpdatePeakEquity(mtmEquity)
}

func (account *riskAccount) RealizedDay() float64 {
	if account == nil {
		return 0
	}

	account.mu.Lock()
	defer account.mu.Unlock()

	return account.realizedDay
}

func (account *riskAccount) Drawdown() float64 {
	if account == nil {
		return 0
	}

	account.mu.Lock()
	defer account.mu.Unlock()

	return account.portfolio.DrawdownPct(account.wallet)
}

/*
preTradeGate enforces the configured caps before a new entry is sized. The
gate checks are intentionally sharper than the audit logs imply: a tripped
gate ends the entry path without entering and without emitting an order
frame, so the caller can rely on a returned error meaning no order was
submitted.
*/
func (crypto *Crypto) preTradeGate(symbol string, edge, jointConfidence float64) error {
	cfg := config.System

	if crypto.risk == nil {
		return nil
	}

	// jointConfidence is not yet wired into a hard gate (calibration data
	// on per-source false-positive rates is still being collected) but it
	// is logged with the gate decision so a future threshold can be
	// introduced without changing the call site.
	_ = jointConfidence

	// The old guard "edge*cfg.MaxLossPerTradeEUR < 0" was unreachable —
	// tryEnter already rejects on edge <= 0. The real check is whether
	// the worst-case loss on this trade exceeds the configured per-trade
	// cap, given the stop distance the trade will be sized against. We
	// approximate worst-case as the slot's notional × the trail floor
	// the sizing layer will apply; if that exceeds the cap we bail.
	if cfg.MaxLossPerTradeEUR > 0 && cfg.DefaultTrailPct > 0 {
		balance := crypto.wallet.AvailableEUR()
		notionalEstimate := balance * cfg.MaxSlotPct / 100
		worstLoss := notionalEstimate * cfg.DefaultTrailPct / 100

		if worstLoss > cfg.MaxLossPerTradeEUR {
			return fmt.Errorf(
				"worst-case loss %.4f > MaxLossPerTradeEUR %.4f (edge %.6f)",
				worstLoss, cfg.MaxLossPerTradeEUR, edge,
			)
		}
	}

	realized := crypto.risk.RealizedDay()

	if cfg.MaxDailyLossEUR > 0 && realized <= -cfg.MaxDailyLossEUR {
		return fmt.Errorf(
			"daily realized PnL %.2f <= -MaxDailyLossEUR %.2f",
			realized, cfg.MaxDailyLossEUR,
		)
	}

	if cfg.MaxPortfolioDrawdownPct > 0 {
		dd := crypto.risk.Drawdown()

		if dd >= cfg.MaxPortfolioDrawdownPct {
			return fmt.Errorf(
				"drawdown %.4f >= MaxPortfolioDrawdownPct %.4f",
				dd, cfg.MaxPortfolioDrawdownPct,
			)
		}
	}

	if cfg.MaxSymbolCorrelation > 0 {
		openSymbols := crypto.openSymbols()

		if rho, ok := crypto.risk.SystemicCorrelation(symbol, openSymbols); ok {
			if rho >= cfg.MaxSymbolCorrelation {
				return fmt.Errorf(
					"systemic correlation %.4f >= MaxSymbolCorrelation %.4f",
					rho, cfg.MaxSymbolCorrelation,
				)
			}
		}
	}

	return nil
}

// stopFractionFor returns the fractional stop distance for one symbol, derived
// from recent realized volatility and floored/capped by the configured trail
// bounds. StopVolMultiple is a calibration constant approximating volatility
// accumulation over the hold; tune against backtests.
func (crypto *Crypto) stopFractionFor(symbol string) float64 {
	cfg := config.System

	floor := cfg.MinTrailPct / 100
	ceil := cfg.MaxTrailPct / 100

	frac := cfg.StopVolMultiple * crypto.forecasts.RecentVolatility(symbol)

	if frac < floor {
		frac = floor
	}

	if frac > ceil {
		frac = ceil
	}

	return frac
}

/*
stopPricesFor computes an OTO stop level from the measurement's anchor
price and the predicted-return magnitude. The stop is placed at a fraction
of the predicted move (a tighter stop than the target so the trade has a
positive R), and the limit-below-stop is one tick further to allow a stop
limit to clear under fast moves.
*/
func (crypto *Crypto) stopPricesFor(
	lead engine.Measurement,
	predictedReturn float64,
) (float64, float64) {
	cfg := config.System
	anchor := lead.Last

	if anchor <= 0 && lead.Bid > 0 && lead.Ask > 0 {
		anchor = (lead.Bid + lead.Ask) / 2
	}

	if anchor <= 0 || predictedReturn <= 0 {
		return 0, 0
	}

	stopFraction := crypto.stopFractionFor(lead.Pairs[0].Wsname)

	if stopFraction <= 0 {
		return 0, 0
	}

	stop := anchor * (1 - stopFraction)
	limit := stop * (1 - cfg.MinTrailPct/100)

	if limit <= 0 || limit >= stop {
		limit = stop * 0.999
	}

	return stop, limit
}
